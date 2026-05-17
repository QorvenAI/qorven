// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/session"
)

func (gw *Gateway) handleListRooms(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"rooms": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT r.id, r.name, r.display_name, r.description, r.is_dm, r.created_at,
		        (SELECT COUNT(*) FROM room_members WHERE room_id = r.id) as member_count,
		        (SELECT COUNT(*) FROM room_messages WHERE room_id = r.id) as message_count
		 FROM rooms r WHERE r.tenant_id = $1 ORDER BY r.created_at DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, name, displayName, desc string
		var isDM bool
		var createdAt interface{}
		var memberCount, msgCount int
		rows.Scan(&id, &name, &displayName, &desc, &isDM, &createdAt, &memberCount, &msgCount)
		list = append(list, map[string]any{"id": id, "name": name, "display_name": displayName, "description": desc, "is_dm": isDM, "created_at": createdAt, "member_count": memberCount, "message_count": msgCount})
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"rooms": list})
}

func (gw *Gateway) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Members     []string `json:"members"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"})
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = body.Name
	}
	var id string
	err := gw.db.Pool.QueryRow(r.Context(), `INSERT INTO rooms (tenant_id, name, display_name, description) VALUES ($1, $2, $3, $4) RETURNING id`,
		defaultTenant, body.Name, body.DisplayName, body.Description).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// Auto-add all Qors as members — batch INSERT to avoid N+1
	if gw.agents != nil {
		agents, _ := gw.agents.List(r.Context(), defaultTenant)
		allMembers := make([]string, 0, len(agents)+len(body.Members))
		seen := map[string]bool{}
		for _, ag := range agents {
			if !seen[ag.ID] {
				allMembers = append(allMembers, ag.ID)
				seen[ag.ID] = true
			}
		}
		for _, agentID := range body.Members {
			if !seen[agentID] {
				allMembers = append(allMembers, agentID)
				seen[agentID] = true
			}
		}
		if len(allMembers) > 0 {
			// Build batch INSERT: INSERT INTO room_members (room_id, agent_id) VALUES ($1,$2),($1,$3),...
			query := "INSERT INTO room_members (room_id, agent_id) VALUES "
			args := []any{id}
			for i, agID := range allMembers {
				if i > 0 {
					query += ","
				}
				args = append(args, agID)
				query += fmt.Sprintf("($1,$%d)", i+2)
			}
			query += " ON CONFLICT DO NOTHING"
			gw.db.Pool.Exec(r.Context(), query, args...)
		}
	} else {
		// No agent store — just add explicit members
		for _, agentID := range body.Members {
			gw.db.Pool.Exec(r.Context(), `INSERT INTO room_members (room_id, agent_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, agentID)
		}
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var name, displayName, desc string
	err := gw.db.Pool.QueryRow(r.Context(), `SELECT name, display_name, description FROM rooms WHERE id = $1`, id).Scan(&name, &displayName, &desc)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	// Get members
	rows, _ := gw.db.Pool.Query(r.Context(), `SELECT a.id, a.agent_key, a.display_name, a.role FROM room_members rm JOIN agents a ON rm.agent_id = a.id WHERE rm.room_id = $1`, id)
	defer rows.Close()
	members := []map[string]string{}
	for rows.Next() {
		var aid, akey, aname string
		var arole *string
		rows.Scan(&aid, &akey, &aname, &arole)
		m := map[string]string{"id": aid, "agent_key": akey, "display_name": aname}
		if arole != nil {
			m["role"] = *arole
		}
		members = append(members, m)
	}
	if members == nil {
		members = []map[string]string{}
	}
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "display_name": displayName, "description": desc, "members": members})
}

func (gw *Gateway) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	gw.db.Pool.Exec(r.Context(), `DELETE FROM rooms WHERE id = $1`, chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleAddRoomMember(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	gw.db.Pool.Exec(r.Context(), `INSERT INTO room_members (room_id, agent_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, chi.URLParam(r, "id"), body.AgentID)
	writeJSON(w, 200, map[string]string{"status": "added"})
}

func (gw *Gateway) handleGetRoomMessages(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"messages": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT rm.id, rm.sender_id, rm.sender_type, rm.content, rm.message_type, rm.reactions, rm.reply_to, rm.created_at,
		        COALESCE(rp.sender_id,'') as reply_sender, COALESCE(rp.content,'') as reply_content
		 FROM room_messages rm LEFT JOIN room_messages rp ON rm.reply_to = rp.id
		 WHERE rm.room_id = $1 ORDER BY rm.created_at DESC LIMIT 50`, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	msgs := []map[string]any{}
	for rows.Next() {
		var id, senderID, senderType, content, msgType, replySender, replyContent string
		var reactions json.RawMessage
		var replyTo *string
		var createdAt interface{}
		rows.Scan(&id, &senderID, &senderType, &content, &msgType, &reactions, &replyTo, &createdAt, &replySender, &replyContent)
		entry := map[string]any{"id": id, "sender_id": senderID, "sender_type": senderType, "content": content, "message_type": msgType, "reactions": reactions, "created_at": createdAt}
		if replyTo != nil {
			entry["reply_to"] = *replyTo
			entry["reply_sender"] = replySender
			entry["reply_content"] = replyContent
		}
		msgs = append(msgs, entry)
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"messages": msgs})
}

func (gw *Gateway) handlePostRoomMessage(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	roomID := chi.URLParam(r, "id")
	var body struct {
		SenderID   string `json:"sender_id"`
		SenderType string `json:"sender_type"`
		Content    string `json:"content"`
		ReplyTo    string `json:"reply_to"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Content == "" {
		writeJSON(w, 400, map[string]string{"error": "content required"})
		return
	}
	if body.SenderType == "" {
		body.SenderType = "user"
	}
	if body.SenderID == "" {
		body.SenderID = "user"
	}
	var id string
	var replyTo *string
	if body.ReplyTo != "" {
		replyTo = &body.ReplyTo
	}
	gw.db.Pool.QueryRow(r.Context(), `INSERT INTO room_messages (room_id, sender_id, sender_type, content, reply_to) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		roomID, body.SenderID, body.SenderType, body.Content, replyTo).Scan(&id)
	// Broadcast to realtime
	gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": body.SenderID, "content": body.Content}})

	// Gap C fix: Room → Memory Bridge
	// Every room message of sufficient length is saved as ScopeTeam memory.
	// This means agents joining a room later start with full project context — they
	// don't need to read the backlog; it's already in their hierarchical memory search.
	if gw.agentLoop != nil && gw.agentLoop.HierarchyMem != nil && len(body.Content) > 40 {
		go func(roomID, senderID, senderType, content string) {
			bgCtx := context.Background()
			entry := fmt.Sprintf("[Room %s] %s (%s): %s", roomID[:8], senderID, senderType, content)
			if _, err := gw.agentLoop.HierarchyMem.SaveTeamMemory(bgCtx, roomID, entry, "room:"+roomID); err != nil {
				slog.Debug("room.memory_bridge.failed", "room", roomID, "error", err)
			}
		}(roomID, body.SenderID, body.SenderType, body.Content)
	}

	// Room @mention handler — trigger mentioned Qors
	// If no @mention, Prime Qor handles it (like a boss in a group chat)
	if gw.agentLoop != nil && gw.agents != nil {
		slog.Info("room.handler.entry", "room", roomID, "content", body.Content[:min(len(body.Content), 50)], "mentions_check", true)
		mentions := mentionRegex.FindAllStringSubmatch(body.Content, -1)

		// No explicit @mention → Coordinator routes the message
		if len(mentions) == 0 && (body.SenderType == "user" || body.SenderType == "") {
			slog.Info("room.coordinator.routing", "room", roomID, "content", body.Content[:min(len(body.Content), 50)])
			go func(roomID, content string) {
				bgCtx := context.Background()
				agents, _ := gw.agents.List(bgCtx, defaultTenant)
				var primeID string
				agentMap := map[string]*agent.Agent{}
				for _, ag := range agents {
					agentMap[ag.AgentKey] = ag
					if ag.Role != nil && *ag.Role == "Lead" {
						primeID = ag.ID
					}
				}
				if primeID == "" && len(agents) > 0 {
					primeID = agents[0].ID
				}
				if primeID == "" {
					return
				}

				// Load room context
				rows, _ := gw.db.Pool.Query(bgCtx,
					`SELECT sender_id, sender_type, content FROM room_messages WHERE room_id = $1 ORDER BY created_at DESC LIMIT 20`, roomID)
				var roomCtx string
				ctxMsgs := []string{}
				if rows != nil {
					for rows.Next() {
						var sid, stype, content string
						rows.Scan(&sid, &stype, &content)
						ctxMsgs = append([]string{fmt.Sprintf("[%s] %s", sid, content)}, ctxMsgs...)
					}
					rows.Close()
				}
				for _, m := range ctxMsgs {
					roomCtx += m + "\n"
				}

				// Step 1: Coordinator decides who should respond
				// Build agent list for the routing prompt
				var agentList string
				for _, ag := range agents {
					role := "Qor"
					if ag.Role != nil {
						role = *ag.Role
					}
					agentList += fmt.Sprintf("- @%s (%s): %s\n", ag.AgentKey, role, ag.DisplayName)
				}

				// Find last Qor that responded in this room
				lastResponder := "prime"
				for i := len(ctxMsgs) - 1; i >= 0; i-- {
					parts := strings.SplitN(ctxMsgs[i], "] ", 2)
					if len(parts) == 2 {
						sender := strings.TrimPrefix(parts[0], "[")
						if sender != "user" && sender != "" {
							lastResponder = sender
							break
						}
					}
				}

				routePrompt := fmt.Sprintf(`You are a message router. Given a user message in a group chat, decide which agent(s) should respond.

Available agents:
%s
Recent conversation context — last Qor that responded: @%s

Rules:
- If the message is general chat/greeting, route to @qorven (the Lead) only
- If the message asks for something specific, route to the most relevant specialist
- If the message says "dm me/it/that/this" or "send it/that", route to @%s (the last Qor that responded)
- Route to at most 2 agents unless the user explicitly asks "everyone"
- If unsure, route to @qorven only

User message: %s

Respond with JSON: {"route_to": ["agent_key"], "reasoning": "brief reason"}`, agentList, lastResponder, lastResponder, content)

				provider := gw.providerReg.Default()
				if provider == nil || len(agents) == 0 {
					return
				}
				routeResp, err := provider.Chat(bgCtx, providers.ChatRequest{
					Model: agents[0].Model,
					Messages: []providers.Message{
						{Role: "user", Content: routePrompt},
					},
					Options: map[string]any{
						"temperature": 0, "max_tokens": 200,
						"response_format": map[string]string{"type": "json_object"},
					},
				})
				if err != nil {
					slog.Error("room.coordinator.route_failed", "error", err)
					return
				}

				// Parse structured routing decision
				routeText := strings.TrimSpace(routeResp.Content)
				routeTo := []string{}
				// Try structured format: {"route_to": ["researcher"]}
				var structured struct {
					RouteTo []string `json:"route_to"`
				}
				if json.Unmarshal([]byte(routeText), &structured) == nil && len(structured.RouteTo) > 0 {
					routeTo = structured.RouteTo
				} else {
					// Fallback: try plain array ["researcher"]
					start := strings.Index(routeText, "[")
					end := strings.LastIndex(routeText, "]")
					if start >= 0 && end > start {
						json.Unmarshal([]byte(routeText[start:end+1]), &routeTo)
					}
				}
				if len(routeTo) == 0 {
					routeTo = []string{"prime"}
				}
				slog.Info("room.coordinator.routed", "room", roomID, "route_to", routeTo)

				// Step 2: Trigger only the routed Qors
				for _, soulKey := range routeTo {
					ag, ok := agentMap[soulKey]
					if !ok {
						continue
					}

					go func(a *agent.Agent) {
						deliveryChannel := agent.DetectDeliveryChannel(content)
						taskMsg := content
						if deliveryChannel == "internal_dm" {
							for _, kw := range []string{"dm me", "message me", "send me a dm", "privately", "in private", "directly dm", "direct dm", "send dm", "via dm", "to my dm", "in dm"} {
								taskMsg = strings.ReplaceAll(strings.ToLower(taskMsg), kw, "")
							}
							taskMsg = strings.TrimSpace(taskMsg)
							if taskMsg == "" || len(taskMsg) < 5 {
								taskMsg = content
							}
						}

						if deliveryChannel == "internal_dm" {
							// === TWO-STEP GENERATE/DELIVER (mentor pattern) ===
							// Step 1: GENERATE — LLM only produces content, nothing else
							genPrompt := fmt.Sprintf("You are generating content for a DM. Output ONLY the raw content the user asked for. No preamble. No greeting. No sign-off. No mention of DM or delivery. Just the content itself.\n\nConversation context:\n%s\nUser request: %s", roomCtx, taskMsg)
							generatedContent, _ := gw.agentLoop.Chat(context.Background(), a.ID, genPrompt)

							// If "dm it/that" — look up last Qor message from room DB
							if generatedContent == "" || len(generatedContent) < 20 {
								var lastContent string
								gw.db.Pool.QueryRow(context.Background(),
									`SELECT content FROM room_messages WHERE room_id = $1 AND sender_type = 'soul' AND content NOT LIKE '%✅%' AND LENGTH(content) > 50 ORDER BY created_at DESC LIMIT 1`, roomID).Scan(&lastContent)
								if lastContent != "" {
									generatedContent = lastContent
								}
							}

							if generatedContent != "" {
								// Step 2: DELIVER — Go code handles delivery, not LLM
								highlight := agent.ExtractHighlight(generatedContent, 120)

								// 2a: Save to DM session
								if gw.sessions != nil {
									sessions, _ := gw.sessions.List(context.Background(), defaultTenant, a.ID, 1)
									var sessID string
									if len(sessions) > 0 {
										sessID = sessions[0].ID
									} else {
										sess, err := gw.sessions.Create(context.Background(), defaultTenant, a.ID, "user", "web")
										if err == nil {
											sessID = sess.ID
										}
									}
									if sessID != "" {
										gw.sessions.AppendMessage(context.Background(), sessID, session.Message{Role: "assistant", Content: generatedContent, Timestamp: time.Now().Unix()}, 0, 0)
										gw.rtHub.Broadcast(realtime.Event{Type: "new_message", Data: map[string]string{
											"session_id": sessID, "agent_id": a.ID, "soul_key": a.AgentKey, "soul_name": a.DisplayName,
											"role": "assistant", "content": generatedContent, "highlight": highlight,
										}})
										slog.Info("room.dm_delivered", "soul", a.AgentKey, "session", sessID, "content_len", len(generatedContent))
										gw.writeNotification(a.ID, a.AgentKey, a.DisplayName, "message", a.DisplayName, highlight, "dm", sessID)
									}
								}

								// 2b: Post hardcoded confirmation in room (NEVER from LLM)
								confirm := fmt.Sprintf("✅ Sent to your DM with %s.", a.DisplayName)
								gw.db.Pool.Exec(context.Background(),
									`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`, roomID, a.AgentKey, confirm)
								gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": a.AgentKey, "content": confirm}})
								gw.rtHub.Broadcast(realtime.Event{Type: "stream_end", Data: map[string]string{
									"soul_id": a.ID, "soul_key": a.AgentKey, "soul_name": a.DisplayName,
									"msg_id": fmt.Sprintf("msg_%s_%d", a.AgentKey, time.Now().UnixMilli()), "room_id": roomID, "highlight": highlight,
								}})
							}
						} else {
							// Regular room response — stream to room
							prompt := fmt.Sprintf("[Room: #%s | You: @%s]\nConversation:\n%s\nUser request: %s", roomID[:8], a.AgentKey, roomCtx, taskMsg)
							msgID := fmt.Sprintf("msg_%s_%d", a.AgentKey, time.Now().UnixMilli())
							gw.rtHub.Broadcast(realtime.Event{Type: "stream_start", Data: map[string]string{
								"soul_id": a.ID, "soul_key": a.AgentKey, "soul_name": a.DisplayName, "msg_id": msgID, "room_id": roomID,
							}})
							chatResp, chatStreamErr := gw.agentLoop.ChatStream(context.Background(), a.ID, prompt, func(delta string) {
								gw.rtHub.Broadcast(realtime.Event{Type: "stream_delta", Data: map[string]string{
									"soul_id": a.ID, "soul_key": a.AgentKey, "msg_id": msgID, "room_id": roomID, "delta": delta,
								}})
							})
							hl := agent.ExtractHighlight(chatResp, 120)
							gw.rtHub.Broadcast(realtime.Event{Type: "stream_end", Data: map[string]string{
								"soul_id": a.ID, "soul_key": a.AgentKey, "soul_name": a.DisplayName, "msg_id": msgID, "room_id": roomID, "highlight": hl,
							}})
							if chatStreamErr != nil {
								slog.Error("room.agent.chat_failed", "agent", a.AgentKey, "error", chatStreamErr)
							}
							if chatResp != "" {
								gw.db.Pool.Exec(context.Background(),
									`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`, roomID, a.AgentKey, chatResp)
								gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": a.AgentKey, "content": chatResp}})
							}
						}
					}(ag)
				}
			}(roomID, body.Content)
		}

		// Explicit @mentions → trigger those specific Qors
		for _, m := range mentions {
			soulKey := m[1]
			go func(key string) {
				agents, _ := gw.agents.List(context.Background(), defaultTenant)
				for _, ag := range agents {
					if !strings.EqualFold(ag.AgentKey, key) {
						continue
					}
					// Load room context (last 20 messages)
					rows, err := gw.db.Pool.Query(context.Background(),
						`SELECT sender_id, sender_type, content FROM room_messages WHERE room_id = $1 ORDER BY created_at DESC LIMIT 20`, roomID)
					if err != nil {
						break
					}
					var roomCtx string
					msgs := []string{}
					for rows.Next() {
						var sid, stype, content string
						rows.Scan(&sid, &stype, &content)
						msgs = append([]string{fmt.Sprintf("[%s] %s", sid, content)}, msgs...)
					}
					rows.Close()
					for _, m := range msgs {
						roomCtx += m + "\n"
					}

					// Detect delivery intent deterministically
					deliveryChannel := agent.DetectDeliveryChannel(body.Content)

					// Strip delivery keywords from prompt so Soul focuses on the task
					taskMsg := body.Content
					for _, kw := range []string{"dm me", "message me", "send me a dm", "privately", "in private", "email me", "send email", "telegram me", "whatsapp me"} {
						taskMsg = strings.ReplaceAll(strings.ToLower(taskMsg), kw, "")
					}
					taskMsg = strings.TrimSpace(taskMsg)
					if taskMsg == "" {
						taskMsg = body.Content
					}

					prompt := fmt.Sprintf("[Room: #%s | You: @%s]\nConversation:\n%s\nUser request: %s\nWrite the complete content. Do not mention delivery — just produce the requested content.", roomID[:8], ag.AgentKey, roomCtx, taskMsg)
					slog.Info("room.mention.stream_start", "soul", ag.AgentKey, "delivery", deliveryChannel)

					// If scheduling request, create cron job deterministically (don't rely on LLM)
					if agent.IsSchedulingRequest(body.Content) {
						expr, task, ok := agent.ParseSchedule(body.Content)
						if ok {
							slog.Info("room.mention.scheduling_deterministic", "soul", ag.AgentKey, "expr", expr, "task", task)
							// Create cron job directly in DB
							var jobID string
							jobName := strings.ReplaceAll(strings.ToLower(ag.AgentKey+"_scheduled"), " ", "_")
							payloadJSON, _ := json.Marshal(map[string]string{"instruction": task, "executor_agent_id": ag.ID})
							gw.db.Pool.QueryRow(context.Background(),
								`INSERT INTO cron_jobs (agent_id, name, cron_expression, payload, executor_agent_id, enabled)
								 VALUES ($1, $2, $3, $4, $5, true) RETURNING id`,
								ag.ID, jobName, expr, payloadJSON, ag.ID).Scan(&jobID)

							humanTime := cronpkg.FormatNextRunExpr(expr)
							confirm := fmt.Sprintf("Got it! I'll send you %s, %s. 👍", task, humanTime)
							gw.db.Pool.Exec(context.Background(),
								`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`, roomID, ag.AgentKey, confirm)
							gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": ag.AgentKey, "content": confirm}})
							break
						}
						// If parsing failed, fall through to LLM with full agent loop
						slog.Info("room.mention.scheduling_llm_fallback", "soul", ag.AgentKey)
						result, _ := gw.agentLoop.Run(context.Background(), agent.RunRequest{
							AgentID: ag.ID, UserMessage: body.Content, Channel: "room",
						}, func(event agent.StreamEvent) {})
						if result != nil && result.Content != "" {
							gw.db.Pool.Exec(context.Background(),
								`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`, roomID, ag.AgentKey, result.Content)
							gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": ag.AgentKey, "content": result.Content}})
						}
						break
					}

					// Generate unique message ID for this Soul's stream
					msgID := fmt.Sprintf("msg_%s_%d", ag.AgentKey, time.Now().UnixMilli())

					// Emit STREAM_START
					gw.rtHub.Broadcast(realtime.Event{Type: "stream_start", Data: map[string]string{
						"soul_id": ag.ID, "soul_key": ag.AgentKey, "soul_name": ag.DisplayName,
						"msg_id": msgID, "room_id": roomID,
					}})

					// Stream token-by-token
					chatResult, chatErr := gw.agentLoop.ChatStream(context.Background(), ag.ID, prompt, func(delta string) {
						gw.rtHub.Broadcast(realtime.Event{Type: "stream_delta", Data: map[string]string{
							"soul_id": ag.ID, "soul_key": ag.AgentKey, "msg_id": msgID, "room_id": roomID, "delta": delta,
						}})
					})

					// Emit STREAM_END
					mentionHighlight := agent.ExtractHighlight(chatResult, 120)
					gw.rtHub.Broadcast(realtime.Event{Type: "stream_end", Data: map[string]string{
						"soul_id": ag.ID, "soul_key": ag.AgentKey, "soul_name": ag.DisplayName, "msg_id": msgID, "room_id": roomID, "highlight": mentionHighlight,
					}})
					slog.Info("room.mention.stream_done", "soul", ag.AgentKey, "content_len", len(chatResult), "error", chatErr)

					if chatResult != "" {
						if deliveryChannel == "internal_dm" {
							// Deliver via DM: save to user's session with this Soul
							if gw.sessions != nil {
								// Find or create a session for this Soul
								sessions, _ := gw.sessions.List(context.Background(), defaultTenant, ag.ID, 1)
								var sessionID string
								if len(sessions) > 0 {
									sessionID = sessions[0].ID
								} else {
									sess, err := gw.sessions.Create(context.Background(), defaultTenant, ag.ID, "user", "web")
									if err == nil {
										sessionID = sess.ID
									}
								}
								if sessionID != "" {
									// Save the content as a message in the DM session
									gw.sessions.AppendMessage(context.Background(), sessionID, session.Message{Role: "assistant", Content: chatResult, Timestamp: time.Now().Unix()}, 0, 0)
									// Notify via WebSocket so UI picks it up
									gw.rtHub.Broadcast(realtime.Event{Type: "new_message", Data: map[string]string{
										"session_id": sessionID, "agent_id": ag.ID, "soul_key": ag.AgentKey, "soul_name": ag.DisplayName, "role": "assistant", "content": chatResult, "highlight": agent.ExtractHighlight(chatResult, 120),
									}})
									slog.Info("room.mention.dm_delivered", "soul", ag.AgentKey, "session", sessionID)
									gw.writeNotification(ag.ID, ag.AgentKey, ag.DisplayName, "message", ag.DisplayName, agent.ExtractHighlight(chatResult, 120), "dm", sessionID)
								}
							}
							// Post confirmation in room
							confirm := fmt.Sprintf("✅ Sent the response to your DM with %s.", ag.DisplayName)
							gw.db.Pool.Exec(context.Background(),
								`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`,
								roomID, ag.AgentKey, confirm)
							gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{
								"room_id": roomID, "sender": ag.AgentKey, "content": confirm,
							}})
						} else {
							// Default: post in room
							gw.db.Pool.Exec(context.Background(),
								`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1, $2, 'soul', $3)`,
								roomID, ag.AgentKey, chatResult)
							gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{
								"room_id": roomID, "sender": ag.AgentKey, "content": chatResult,
							}})
						}
					}
					break
				}
			}(soulKey)
		}
	}

	writeJSON(w, 201, map[string]string{"id": id})
}
