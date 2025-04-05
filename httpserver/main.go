package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	headscaleSocket = "/var/run/headscale/headscale.sock"
	defaultUser     = "default"
)

type PreAuthKeyRequest struct {
	User      string   `json:"user"`
	Reusable  bool     `json:"reusable"`
	Ephemeral bool     `json:"ephemeral"`
	Tags      []string `json:"tags"`
	Expiry    string   `json:"expiry"`
}

type PreAuthKeyResponse struct {
	Key string `json:"key"`
}

func createDefaultUser(client v1.HeadscaleServiceClient) error {
	ctx := context.Background()
	
	// Check if user exists
	listReq := &v1.ListUsersRequest{}
	listResp, err := client.ListUsers(ctx, listReq)
	if err != nil {
		return err
	}

	// If user doesn't exist, create it
	userExists := false
	for _, user := range listResp.GetUsers() {
		if user.GetName() == defaultUser {
			userExists = true
			break
		}
	}

	if !userExists {
		createReq := &v1.CreateUserRequest{
			Name: defaultUser,
		}
		_, err := client.CreateUser(ctx, createReq)
		if err != nil {
			return err
		}
		log.Printf("Created default user: %s", defaultUser)
	} else {
		log.Printf("Default user %s already exists", defaultUser)
	}

	return nil
}

func createPreAuthKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PreAuthKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Connect to headscale via Unix socket
	conn, err := grpc.Dial("unix://"+headscaleSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		http.Error(w, "Failed to connect to headscale", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	client := v1.NewHeadscaleServiceClient(conn)

	// Parse expiry duration
	expiry, err := time.ParseDuration(req.Expiry)
	if err != nil {
		http.Error(w, "Invalid expiry duration", http.StatusBadRequest)
		return
	}

	// Create preauth key request
	request := &v1.CreatePreAuthKeyRequest{
		User:      req.User,
		Reusable:  req.Reusable,
		Ephemeral: req.Ephemeral,
		AclTags:   req.Tags,
		Expiration: timestamppb.New(time.Now().Add(expiry)),
	}

	// Call headscale to create preauth key
	response, err := client.CreatePreAuthKey(context.Background(), request)
	if err != nil {
		log.Printf("Failed to create preauth key: %v", err)
		http.Error(w, "Failed to create preauth key", http.StatusInternalServerError)
		return
	}

	// Return the key
	resp := PreAuthKeyResponse{
		Key: response.GetPreAuthKey().GetKey(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	// Check if headscale socket exists
	if _, err := os.Stat(headscaleSocket); os.IsNotExist(err) {
		log.Fatalf("Headscale socket not found at %s", headscaleSocket)
	}

	// Connect to headscale
	conn, err := grpc.Dial("unix://"+headscaleSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to headscale: %v", err)
	}
	defer conn.Close()

	client := v1.NewHeadscaleServiceClient(conn)

	// Create default user
	if err := createDefaultUser(client); err != nil {
		log.Fatalf("Failed to create default user: %v", err)
	}

	http.HandleFunc("/preauthkey", createPreAuthKey)
	log.Println("Server starting on :48080...")
	log.Fatal(http.ListenAndServe(":48080", nil))
} 
