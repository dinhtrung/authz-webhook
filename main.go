package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gofiber/fiber/v3"
)

// Configuration constants
const (
	CacheTTL = 60 * time.Minute
)

var (
	listenAddr     string
	kratosAdminURL string
	helpRequested  bool
)

func init() {
	// 1. Apply defaults
	listenAddr = ":5000"
	kratosAdminURL = "http://127.0.0.1:4434/admin/identities"

	// 2. Override with Environment Variables if set
	if v := os.Getenv("LISTEN"); v != "" {
		listenAddr = v
	}
	if v := os.Getenv("KRATOS_ADMIN_URL"); v != "" {
		kratosAdminURL = v
	}

	// 3. Define flags
	flag.StringVar(&listenAddr, "listen", listenAddr, "Listen address")
	flag.StringVar(&kratosAdminURL, "kratos-url", kratosAdminURL, "Kratos Admin URL")
	flag.BoolVar(&helpRequested, "help", false, "Show this help message")
	flag.BoolVar(&helpRequested, "h", false, "Show this help message (shorthand)")
}

// OathkeeperPayload represents the incoming request shape from Oathkeeper
type OathkeeperPayload struct {
	IdentityID string `json:"identity_id"`
}

// KratosIdentity represents the truncated response from Kratos Admin API
type KratosIdentity struct {
	ID            string `json:"id"`
	MetadataAdmin struct {
		IsAdmin bool `json:"IsAdmin"`
	} `json:"metadata_admin"`
}

var db *badger.DB

func main() {
	// Parse flags (allows CLI to override env vars)
	flag.Parse()

	if helpRequested {
		fmt.Println("Authorization Webhook for Ory Oathkeeper")
		fmt.Println("")
		fmt.Println("Usage:")
		fmt.Println("  -listen string")
		fmt.Println("        Listen address (default \":5000\")")
		fmt.Println("  -kratos-url string")
		fmt.Println("        Kratos Admin URL (default \"http://127.0.0.1:4434/admin/identities\")")
		fmt.Println("  -help")
		fmt.Println("        Show this help message")
		fmt.Println("  -h")
		fmt.Println("        Show this help message (shorthand)")
		os.Exit(0)
	}

	// 1. Initialize BadgerDB fully in-memory
	opts := badger.DefaultOptions("").WithInMemory(true).WithLoggingLevel(badger.ERROR)
	var err error
	db, err = badger.Open(opts)
	if err != nil {
		log.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	// 2. Initialize Fiber App
	app := fiber.New(fiber.Config{
		AppName: "Oathkeeper Authz Webhook",
	})

	// 3. Define the validation route called by Oathkeeper remote_json
	app.Post("/validate-role", handleAuthzValidation)

	log.Printf("Authorization webhook listening on %s", listenAddr)
	log.Fatal(app.Listen(listenAddr))
}

func handleAuthzValidation(c fiber.Ctx) error {
	var payload OathkeeperPayload
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload structure"})
	}

	if payload.IdentityID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Missing identity_id"})
	}

	// Check internal Badger memory cache
	isAdmin, cacheHit := checkCache(payload.IdentityID)
	if cacheHit {
		if isAdmin {
			return c.SendStatus(http.StatusOK) // 200 OK -> Access Granted
		}
		return c.SendStatus(http.StatusForbidden) // 403 -> Access Denied
	}

	// Cache Miss: Query Kratos Admin API directly
	kratosURL := fmt.Sprintf("%s/%s", kratosAdminURL, payload.IdentityID)
	resp, err := http.Get(kratosURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("Error fetching Kratos Identity %s: %v", payload.IdentityID, err)
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Authorization verification failed"})
	}
	defer resp.Body.Close()

	var identity KratosIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse Kratos structure"})
	}

	// Extract administrative boolean state
	isUserAdmin := identity.MetadataAdmin.IsAdmin

	// Set value into in-memory BadgerDB cache with a short TTL
	setCache(payload.IdentityID, isUserAdmin)

	if isUserAdmin {
		return c.SendStatus(http.StatusOK)
	}
	return c.SendStatus(http.StatusForbidden)
}

// --- BADGERDB OPERATIONS ---

func checkCache(identityID string) (bool, bool) {
	var isAdmin bool
	var found bool = false

	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(identityID))
		if err != nil {
			return err // Key not found or expired
		}

		return item.Value(func(val []byte) error {
			if len(val) > 0 && val[0] == 1 {
				isAdmin = true
			} else {
				isAdmin = false
			}
			found = true
			return nil
		})
	})

	if err != nil {
		return false, false
	}
	return isAdmin, found
}

func setCache(identityID string, isAdmin bool) {
	err := db.Update(func(txn *badger.Txn) error {
		val := []byte{0}
		if isAdmin {
			val = []byte{1}
		}

		e := badger.NewEntry([]byte(identityID), val).WithTTL(CacheTTL)
		return txn.SetEntry(e)
	})
	if err != nil {
		log.Printf("Badger cache write failure for %s: %v", identityID, err)
	}
}
