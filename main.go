package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	client       *whatsmeow.Client
	container    *sqlstore.Container
	db           *sql.DB
	agentBaseURL string
	serverBaseURL string
	serverPort   string
	mediaMap     sync.Map
)

type MessageContent struct {
	Type        string `json:"type"`
	Body        string `json:"body,omitempty"`
	Caption     string `json:"caption,omitempty"`
	Mimetype    string `json:"mimetype,omitempty"`
	DownloadURL string `json:"downloadURL,omitempty"`
}

type AgentMessage struct {
	MessageID string         `json:"messageID"`
	Timestamp time.Time      `json:"timestamp"`
	SenderJID string         `json:"senderJID"`
	ChatJID   string         `json:"chatJID"`
	IsGroup   bool           `json:"isGroup"`
	IsFromMe  bool           `json:"isFromMe"`
	Content   MessageContent `json:"content"`
}

type SendMessageRequest struct {
	JID     string `json:"jid"`
	Message string `json:"message"`
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		fmt.Println("✅ Login successful")
		postJSON(agentBaseURL+"/api/status", map[string]string{"status": "logged_in"})
	case *events.Disconnected:
		fmt.Println("🔌 Disconnected")
		postJSON(agentBaseURL+"/api/status", map[string]string{"status": "disconnected"})
	case *events.Message:
		// Full event debug
		fmt.Printf("DEBUG FULL EVENT: %+v\n", v)
		// Raw message debug
		fmt.Printf("DEBUG RAW MESSAGE: %+v\n", v.Message)

		fmt.Printf("Message received: From=%s, IsGroup=%t\n", v.Info.Sender, v.Info.IsGroup)

		isFromMe := false
		if client != nil && client.Store != nil && client.Store.ID != nil {
			// Check if the message sender is the logged-in user
			isFromMe = v.Info.Sender.User == client.Store.ID.User
		}

		agentMsg := AgentMessage{
			MessageID: v.Info.ID,
			Timestamp: v.Info.Timestamp,
			SenderJID: v.Info.Sender.String(),
			ChatJID:   v.Info.Chat.String(),
			IsGroup:   v.Info.IsGroup,
			IsFromMe:  isFromMe,
		}
		msg := v.Message
		// Improved extraction for all major WhatsApp message types
		switch {
		case msg.GetSenderKeyDistributionMessage() != nil:
			fmt.Println("Ignoring sender key distribution message")
			return // Ignore these technical messages
		case msg.GetConversation() != "":
			agentMsg.Content.Type = "text"
			agentMsg.Content.Body = msg.GetConversation()
		case msg.GetExtendedTextMessage() != nil && msg.GetExtendedTextMessage().GetText() != "":
			agentMsg.Content.Type = "text"
			agentMsg.Content.Body = msg.GetExtendedTextMessage().GetText()
		case msg.GetImageMessage() != nil:
			agentMsg.Content.Type = "image"
			agentMsg.Content.Caption = msg.GetImageMessage().GetCaption()
			agentMsg.Content.Mimetype = msg.GetImageMessage().GetMimetype()
			agentMsg.Content.DownloadURL = fmt.Sprintf("%s/api/download/%s", serverBaseURL, v.Info.ID)
			mediaMap.Store(v.Info.ID, msg.GetImageMessage())
		case msg.GetVideoMessage() != nil:
			agentMsg.Content.Type = "video"
			agentMsg.Content.Caption = msg.GetVideoMessage().GetCaption()
			agentMsg.Content.Mimetype = msg.GetVideoMessage().GetMimetype()
			agentMsg.Content.DownloadURL = fmt.Sprintf("%s/api/download/%s", serverBaseURL, v.Info.ID)
			mediaMap.Store(v.Info.ID, msg.GetVideoMessage())
		case msg.GetDocumentMessage() != nil:
			agentMsg.Content.Type = "document"
			agentMsg.Content.Caption = msg.GetDocumentMessage().GetCaption()
			agentMsg.Content.Mimetype = msg.GetDocumentMessage().GetMimetype()
			agentMsg.Content.DownloadURL = fmt.Sprintf("%s/api/download/%s", serverBaseURL, v.Info.ID)
			mediaMap.Store(v.Info.ID, msg.GetDocumentMessage())
		case msg.GetAudioMessage() != nil:
			agentMsg.Content.Type = "audio"
			agentMsg.Content.Mimetype = msg.GetAudioMessage().GetMimetype()
			agentMsg.Content.DownloadURL = fmt.Sprintf("%s/api/download/%s", serverBaseURL, v.Info.ID)
			mediaMap.Store(v.Info.ID, msg.GetAudioMessage())
		case msg.GetStickerMessage() != nil:
			agentMsg.Content.Type = "sticker"
			agentMsg.Content.DownloadURL = fmt.Sprintf("%s/api/download/%s", serverBaseURL, v.Info.ID)
			mediaMap.Store(v.Info.ID, msg.GetStickerMessage())
		case msg.GetContactMessage() != nil:
			agentMsg.Content.Type = "contact"
			agentMsg.Content.Body = msg.GetContactMessage().GetDisplayName()
		case msg.GetButtonsMessage() != nil:
			agentMsg.Content.Type = "buttons"
			agentMsg.Content.Body = msg.GetButtonsMessage().GetContentText()
		case msg.GetListMessage() != nil:
			agentMsg.Content.Type = "list"
			agentMsg.Content.Body = msg.GetListMessage().GetDescription()
		default:
			agentMsg.Content.Type = "unsupported"
			agentMsg.Content.Body = "Message type not supported by PoC server."
		}

		// Attach chat history (last 10 messages, sorted chronologically)
		history, err := getRecentChatHistory(v.Info.Chat.String(), 10)
		if err != nil {
			fmt.Printf("Error fetching chat history: %v\n", err)
		}

		// The history is already processed by getRecentChatHistory -> getMessages -> executeMessageQuery
		// No further processing is needed here. The data is consistent.

		payload := map[string]interface{}{
			"message": agentMsg,
			"history": history,
		}
		postJSON(agentBaseURL+"/api/message", payload)

		// Store the message after processing
		serializedMsg, err := proto.Marshal(v.Message)
		if err != nil {
			fmt.Printf("Failed to serialize message for storage: %v\n", err)
		} else {
			// Using a goroutine to avoid blocking the event handler
			go func() {
				if err := storeMessage(v.Info.ID, v.Info.Chat, v.Info.Sender, serializedMsg, v.Info.Timestamp); err != nil {
					fmt.Printf("Failed to store message: %v\n", err)
				}
			}()
		}
	}
}

// getRecentChatHistory fetches the last N messages for a chat and sorts them chronologically (ASC).
func getRecentChatHistory(chatJID string, limit int) ([]map[string]interface{}, error) {
	// Use the main getMessages function to ensure consistent output and logic.
	// No sender, start time, or end time filters are applied.
	return getMessages(chatJID, "", limit, 0, 0)
}

// getMessages fetches messages from the database with optional filters.
// It returns the most recent messages matching the criteria, sorted chronologically (ASC).
func getMessages(chatJID, senderJID string, limit int, startTime, endTime int64) ([]map[string]interface{}, error) {
	var baseQuery strings.Builder
	var args []interface{}

	// Base selection and filtering
	baseQuery.WriteString("SELECT message_id, timestamp, sender_jid, chat_jid, message_content FROM messages WHERE 1=1")
	if chatJID != "" {
		baseQuery.WriteString(" AND chat_jid = ?")
		args = append(args, chatJID)
	}
	if senderJID != "" {
		baseQuery.WriteString(" AND sender_jid = ?")
		args = append(args, senderJID)
	}
	if startTime > 0 {
		baseQuery.WriteString(" AND timestamp >= ?")
		args = append(args, startTime)
	}
	if endTime > 0 {
		baseQuery.WriteString(" AND timestamp <= ?")
		args = append(args, endTime)
	}

	var finalQuery string
	if limit > 0 {
		// Subquery to get the N most recent messages, then sort them chronologically.
		// The alias for the subquery is required by some SQL dialects, and is good practice.
		finalQuery = fmt.Sprintf("SELECT * FROM (%s ORDER BY timestamp DESC LIMIT ?) sub ORDER BY timestamp ASC", baseQuery.String())
		args = append(args, limit)
	} else {
		// No limit, just get all messages in chronological order.
		finalQuery = baseQuery.String() + " ORDER BY timestamp ASC"
	}

	return executeMessageQuery(finalQuery, args...)
}

// executeMessageQuery runs a given query and processes the results.
func executeMessageQuery(query string, args ...interface{}) ([]map[string]interface{}, error) {
	messages := []map[string]interface{}{}
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, sender, chatJID string
		var content []byte
		var timestamp int64

		if err := rows.Scan(&id, &timestamp, &sender, &chatJID, &content); err != nil {
			fmt.Printf("Error scanning message row: %v\n", err)
			continue
		}

		// Convert timestamp to IST string
		ist, _ := time.LoadLocation("Asia/Kolkata")
		formattedTime := time.Unix(timestamp, 0).In(ist).Format("Mon, 02 Jan 2006 15:04:05 MST")

		parsedSenderJID, _ := types.ParseJID(sender)
		isFromMe := false
		if client.Store != nil && client.Store.ID != nil {
			isFromMe = parsedSenderJID.User == client.Store.ID.User
		}

		msgMap := map[string]interface{}{
			"id":        id,
			"timestamp": formattedTime,
			"sender":    sender,
			"chat":      chatJID,
			"isFromMe":  isFromMe,
		}

		var protoMsg waProto.Message
		if err := proto.Unmarshal(content, &protoMsg); err == nil {
			msgContent := make(map[string]string)
			msgContent["type"] = "unsupported"
			msgContent["body"] = "Message type not supported for content extraction."

			switch {
			case protoMsg.GetConversation() != "":
				msgContent["type"] = "text"
				msgContent["body"] = protoMsg.GetConversation()
			case protoMsg.GetExtendedTextMessage() != nil:
				msgContent["type"] = "text"
				msgContent["body"] = protoMsg.GetExtendedTextMessage().GetText()
			case protoMsg.GetImageMessage() != nil:
				msgContent["type"] = "image"
				msgContent["body"] = protoMsg.GetImageMessage().GetCaption()
			case protoMsg.GetVideoMessage() != nil:
				msgContent["type"] = "video"
				msgContent["body"] = protoMsg.GetVideoMessage().GetCaption()
			case protoMsg.GetDocumentMessage() != nil:
				msgContent["type"] = "document"
				msgContent["body"] = protoMsg.GetDocumentMessage().GetCaption()
			case protoMsg.GetButtonsMessage() != nil:
				msgContent["type"] = "buttons"
				msgContent["body"] = protoMsg.GetButtonsMessage().GetContentText()
			case protoMsg.GetListMessage() != nil:
				msgContent["type"] = "list"
				msgContent["body"] = protoMsg.GetListMessage().GetDescription()
			}
			msgMap["content"] = msgContent
		} else {
			msgMap["content"] = map[string]string{"error": "Failed to parse message content"}
		}

		messages = append(messages, msgMap)
	}
	return messages, nil
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		http.Error(w, "Invalid JID: "+err.Error(), http.StatusBadRequest)
		return
	}
	msg := &waProto.Message{Conversation: &req.Message}
	resp, err := client.SendMessage(context.Background(), jid, msg)
	if err != nil {
		http.Error(w, "Failed to send message: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Message sent successfully! (ID: %s)", resp.ID)
}

func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	chatJID := queryParams.Get("chat_jid")
	senderJID := queryParams.Get("sender_jid")
	limitStr := queryParams.Get("limit")
	startTimeStr := queryParams.Get("start_time")
	endTimeStr := queryParams.Get("end_time")

	limit := 10 // Default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	var startTime, endTime int64
	if startTimeStr != "" {
		if parsedTime, err := strconv.ParseInt(startTimeStr, 10, 64); err == nil {
			startTime = parsedTime
		}
	}
	if endTimeStr != "" {
		if parsedTime, err := strconv.ParseInt(endTimeStr, 10, 64); err == nil {
			endTime = parsedTime
		}
	}

	messages, err := getMessages(chatJID, senderJID, limit, startTime, endTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve messages: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	messageID := vars["messageID"]
	mediaData, ok := mediaMap.Load(messageID)
	if !ok {
		http.Error(w, "Media not found or expired", http.StatusNotFound)
		return
	}
	downloadable, ok := mediaData.(whatsmeow.DownloadableMessage)
	if !ok {
		http.Error(w, "Internal server error: stored media is not downloadable", http.StatusInternalServerError)
		return
	}
	data, err := client.Download(context.Background(), downloadable)
	if err != nil {
		http.Error(w, "Failed to download media: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Write(data)
}

func postJSON(url string, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Error marshalling JSON for %s: %v\n", url, err)
		return
	}
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

func createMessagesTable() error {
	if db == nil {
		return fmt.Errorf("database connection is not initialized")
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		message_id TEXT PRIMARY KEY,
		chat_jid TEXT NOT NULL,
		sender_jid TEXT NOT NULL,
		message_content BLOB,
		timestamp INTEGER
	)`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

func storeMessage(msgID string, chatJID, senderJID types.JID, content []byte, timestamp time.Time) error {
	if db == nil {
		fmt.Println("storeMessage: Database connection is nil")
		return fmt.Errorf("database connection is not initialized")
	}
	fmt.Printf("storeMessage: Preparing to insert message ID %s\n", msgID)

	stmt, err := db.Prepare("INSERT INTO messages (message_id, chat_jid, sender_jid, message_content, timestamp) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		fmt.Printf("storeMessage: Failed to prepare statement: %v\n", err)
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(msgID, chatJID.String(), senderJID.String(), content, timestamp.Unix())
	if err != nil {
		fmt.Printf("storeMessage: Failed to execute statement for message ID %s: %v\n", msgID, err)
		return fmt.Errorf("failed to execute statement: %w", err)
	}
	fmt.Printf("Successfully stored message %s from %s in chat %s\n", msgID, senderJID.String(), chatJID.String())
	return nil
}

func startAPIServer() {
	router := mux.NewRouter()
	router.HandleFunc("/api/send", handleSendMessage).Methods("POST")
	router.HandleFunc("/api/messages", handleGetMessages).Methods("GET")
	router.HandleFunc("/api/download/{messageID}", handleDownload).Methods("GET")
	
	// Use environment variables for server configuration
	serverPort = os.Getenv("PORT")
	if serverPort == "" {
		serverPort = "8080"
	}
	
	serverBaseURL = os.Getenv("SERVER_BASE_URL")
	if serverBaseURL == "" {
		serverBaseURL = fmt.Sprintf("http://localhost:%s", serverPort)
	}
	
	fmt.Printf("Starting API server on %s\n", serverBaseURL)
	if err := http.ListenAndServe(":"+serverPort, router); err != nil {
		fmt.Fprintf(os.Stderr, "API server error: %v\n", err)
	}
}

func main() {
	log := waLog.Stdout("Main", "INFO", true)
	if err := godotenv.Load(); err != nil {
		log.Warnf("Could not load .env file, relying on environment variables: %v", err)
	}
	agentBaseURL = os.Getenv("DUMMY_AGENT_BASE_URL")
	if agentBaseURL == "" {
		panic("DUMMY_AGENT_BASE_URL environment variable not set.")
	}
	dbLog := waLog.Stdout("Database", "INFO", true)

	var err error
	db, err = sql.Open("sqlite3", "file:whatsapp.db?_foreign_keys=on")
	if err != nil {
		panic(fmt.Sprintf("Failed to open database: %v", err))
	}
	defer db.Close()

	// Make sure the table is created before it's used
	if err := createMessagesTable(); err != nil {
		panic(fmt.Sprintf("Failed to create messages table: %v", err))
	}

	container = sqlstore.NewWithDB(db, "sqlite3", dbLog)

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(fmt.Sprintf("Failed to get device from store: %v", err))
	}

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		fmt.Println("No session found. Starting QR login...")
		qrChan, _ := client.GetQRChannel(context.Background())
		if err := client.Connect(); err != nil {
			panic(fmt.Sprintf("Failed to connect: %v", err))
		}
		for qr := range qrChan {
			fmt.Printf("QR code string received. Pushing to agent at %s/api/qr\n", agentBaseURL)
			postJSON(agentBaseURL+"/api/qr", map[string]string{"qr": qr.Code})
		}
	} else {
		fmt.Println("Previous session found. Attempting to connect...")
		if err := client.Connect(); err != nil {
			panic(fmt.Sprintf("Failed to connect with existing session: %v. Please delete whatsapp.db and try again.", err))
		}
	}
	go startAPIServer()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	fmt.Println("\nShutting down...")
	client.Disconnect()
}
