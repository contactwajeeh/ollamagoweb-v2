package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/contactwajeeh/ollamagoweb-v2/mcp"
	"github.com/go-chi/chi"
)

type MCPServerHandler struct {
	*chi.Mux
	db *sql.DB
}

func NewMCPServerHandler(db *sql.DB) *MCPServerHandler {
	h := &MCPServerHandler{
		Mux: chi.NewMux(),
		db:  db,
	}
	h.initRoutes()
	return h
}

func (h *MCPServerHandler) initRoutes() {
	h.Get("/", h.listServers)
	h.Post("/", h.createServer)
	h.Put("/{id}", h.updateServer)
	h.Delete("/{id}", h.deleteServer)
	h.Get("/{id}/tools", h.getServerTools)
	h.Get("/tools", h.getAllTools)
}

func (h *MCPServerHandler) listServers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, server_type, endpoint_url, command, args, env_vars, is_enabled, created_at
		FROM mcp_servers
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Println("Error fetching MCP servers:", err)
		http.Error(w, "Failed to fetch servers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var servers []MCPServerResponse
	for rows.Next() {
		var s MCPServerResponse
		var endpointURL, command, args, envVars sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &s.ServerType, &endpointURL, &command, &args, &envVars, &s.IsEnabled, &s.CreatedAt); err != nil {
			log.Println("Error scanning MCP server:", err)
			continue
		}
		s.EndpointURL = endpointURL.String
		s.Command = command.String
		s.Args = args.String
		s.EnvVars = envVars.String
		servers = append(servers, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (h *MCPServerHandler) createServer(w http.ResponseWriter, r *http.Request) {
	var req MCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Server name is required", http.StatusBadRequest)
		return
	}

	if req.ServerType != "http" && req.ServerType != "stdio" {
		http.Error(w, "Server type must be 'http' or 'stdio'", http.StatusBadRequest)
		return
	}

	if req.ServerType == "http" && req.EndpointURL == "" {
		http.Error(w, "Endpoint URL is required for HTTP servers", http.StatusBadRequest)
		return
	}

	if req.ServerType == "stdio" && req.Command == "" {
		http.Error(w, "Command is required for stdio servers", http.StatusBadRequest)
		return
	}

	result, err := h.db.Exec(`
		INSERT INTO mcp_servers (name, server_type, endpoint_url, command, args, env_vars, is_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.ServerType, req.EndpointURL, req.Command, req.Args, req.EnvVars, 1)
	if err != nil {
		log.Println("Error creating MCP server:", err)
		http.Error(w, "Failed to create server", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   id,
		"name": req.Name,
	})
}

func (h *MCPServerHandler) updateServer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	var req MCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err = h.db.Exec(`
		UPDATE mcp_servers
		SET name = ?, server_type = ?, endpoint_url = ?, command = ?, args = ?, env_vars = ?, is_enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.Name, req.ServerType, req.EndpointURL, req.Command, req.Args, req.EnvVars, req.IsEnabled, id)
	if err != nil {
		log.Println("Error updating MCP server:", err)
		http.Error(w, "Failed to update server", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id})
}

func (h *MCPServerHandler) deleteServer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	_, err = h.db.Exec("DELETE FROM mcp_servers WHERE id = ?", id)
	if err != nil {
		log.Println("Error deleting MCP server:", err)
		http.Error(w, "Failed to delete server", http.StatusInternalServerError)
		return
	}

	mcp.GetMCPClient().DisconnectServer(id)

	w.WriteHeader(http.StatusOK)
}

func (h *MCPServerHandler) getServerTools(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid server ID", http.StatusBadRequest)
		return
	}

	var server mcp.MCPServer
	err = h.db.QueryRow(`
		SELECT id, name, server_type, endpoint_url, command, args, env_vars, is_enabled
		FROM mcp_servers WHERE id = ?
	`, id).Scan(&server.ID, &server.Name, &server.ServerType, &server.EndpointURL, &server.Command, &server.Args, &server.EnvVars, &server.IsEnabled)
	if err == sql.ErrNoRows {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error fetching server:", err)
		http.Error(w, "Failed to fetch server", http.StatusInternalServerError)
		return
	}

	if !server.IsEnabled {
		http.Error(w, "Server is disabled", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tools, err := mcp.GetMCPClient().GetAllEnabledTools(ctx, []*mcp.MCPServer{&server})
	if err != nil {
		log.Println("Error fetching tools:", err)
		http.Error(w, "Failed to fetch tools", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tools)
}

func (h *MCPServerHandler) getAllTools(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, server_type, endpoint_url, command, args, env_vars, is_enabled
		FROM mcp_servers WHERE is_enabled = 1
	`)
	if err != nil {
		log.Println("Error fetching MCP servers:", err)
		http.Error(w, "Failed to fetch servers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var servers []*mcp.MCPServer
	for rows.Next() {
		var server mcp.MCPServer
		if err := rows.Scan(&server.ID, &server.Name, &server.ServerType, &server.EndpointURL, &server.Command, &server.Args, &server.EnvVars, &server.IsEnabled); err != nil {
			log.Println("Error scanning MCP server:", err)
			continue
		}
		servers = append(servers, &server)
	}

	ctx := r.Context()
	tools, err := mcp.GetMCPClient().GetAllEnabledTools(ctx, servers)
	if err != nil {
		log.Println("Error fetching tools:", err)
		http.Error(w, "Failed to fetch tools", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": tools,
		"count": len(tools),
	})
}

type MCPServerRequest struct {
	Name        string `json:"name"`
	ServerType  string `json:"server_type"`
	EndpointURL string `json:"endpoint_url,omitempty"`
	Command     string `json:"command,omitempty"`
	Args        string `json:"args,omitempty"`
	EnvVars     string `json:"env_vars,omitempty"`
	IsEnabled   bool   `json:"is_enabled"`
}

type MCPServerResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ServerType  string `json:"server_type"`
	EndpointURL string `json:"endpoint_url,omitempty"`
	Command     string `json:"command,omitempty"`
	Args        string `json:"args,omitempty"`
	EnvVars     string `json:"env_vars,omitempty"`
	IsEnabled   bool   `json:"is_enabled"`
	CreatedAt   string `json:"created_at,omitempty"`
}
