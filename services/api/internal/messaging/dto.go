package messaging

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/google/uuid"
)

// timeFormat is the RFC3339 layout used across the API for timestamps.
const timeFormat = "2006-01-02T15:04:05Z07:00"

// Content bounds for a single message. The byte bound is checked first so a
// hostile payload is rejected without decoding it.
const (
	maxTextLength     = 4000
	maxContentByte    = 8 * 1024
	maxFileNameLength = 255
)

// UserBriefResponse is the peer profile embedded in conversation payloads. It
// is a public subset: no email, and presence derived on the server.
type UserBriefResponse struct {
	ID            string  `json:"id"`
	Username      *string `json:"username"`
	DisplayName   string  `json:"display_name"`
	AvatarURL     *string `json:"avatar_url"`
	StatusMessage *string `json:"status_message"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"last_seen_at"`
}

// toUserBrief maps a profile; `online` is supplied by the caller because it
// depends on the configured presence window.
func toUserBrief(p ProfileBrief, online bool) UserBriefResponse {
	resp := UserBriefResponse{
		ID:            p.ID.String(),
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarURL,
		StatusMessage: p.StatusMessage,
		Online:        online,
	}
	if p.LastSeenAt != nil {
		s := p.LastSeenAt.UTC().Format(timeFormat)
		resp.LastSeenAt = &s
	}
	return resp
}

// MessageResponse is one message as returned by the API and broadcast over the
// realtime channel.
type MessageResponse struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	SenderID       string          `json:"sender_id"`
	SequenceNo     int64           `json:"sequence_no"`
	ClientMsgID    *string         `json:"client_msg_id"`
	MessageType    string          `json:"message_type"`
	Content        json.RawMessage `json:"content"`
	Status         string          `json:"status"`
	CreatedAt      string          `json:"created_at"`
	EditedAt       *string         `json:"edited_at"`
	DeletedAt      *string         `json:"deleted_at"`
	// Phase 4: advanced-messaging metadata.
	ReplyTo       *ReplyPreview      `json:"reply_to"`
	ForwardedFrom *string            `json:"forwarded_from"`
	PinnedAt      *string            `json:"pinned_at"`
	Reactions     []ReactionResponse `json:"reactions"`
}

// ReplyPreview is the compact rendering of the message a reply quotes. `text`
// is a plain-text preview for list rendering; media replies show their type.
type ReplyPreview struct {
	ID          string          `json:"id"`
	SenderID    string          `json:"sender_id"`
	MessageType string          `json:"message_type"`
	Content     json.RawMessage `json:"content"`
	Deleted     bool            `json:"deleted"`
	Text        string          `json:"text"`
}

// ReactionResponse is one emoji's aggregate on a message.
type ReactionResponse struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Mine  bool     `json:"mine"`
	Users []string `json:"users"`
}

// ToMessageResponse renders a message for a caller whose identity is not
// relevant (conversation previews, socket acks); reaction "mine" is false.
func ToMessageResponse(m Message) MessageResponse {
	return ToMessageResponseFor(m, uuid.Nil)
}

// ToMessageResponseFor renders a message, marking the viewer's own reactions.
func ToMessageResponseFor(m Message, viewer uuid.UUID) MessageResponse {
	resp := MessageResponse{
		ID:             m.ID.String(),
		ConversationID: m.ConversationID.String(),
		SenderID:       m.SenderID.String(),
		SequenceNo:     m.SequenceNo,
		ClientMsgID:    m.ClientMsgID,
		MessageType:    m.MessageType,
		Content:        m.Content,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt.UTC().Format(timeFormat),
		Reactions:      reactionsResponse(m.Reactions, viewer),
	}
	if m.EditedAt != nil {
		s := m.EditedAt.UTC().Format(timeFormat)
		resp.EditedAt = &s
	}
	if m.DeletedAt != nil {
		s := m.DeletedAt.UTC().Format(timeFormat)
		resp.DeletedAt = &s
	}
	if m.ForwardedFrom != nil {
		s := m.ForwardedFrom.String()
		resp.ForwardedFrom = &s
	}
	if m.PinnedAt != nil {
		s := m.PinnedAt.UTC().Format(timeFormat)
		resp.PinnedAt = &s
	}
	if m.ReplyTo != nil {
		resp.ReplyTo = replyPreview(m.ReplyTo)
	}
	return resp
}

// replyPreview renders the quoted message a reply references.
func replyPreview(m *Message) *ReplyPreview {
	p := &ReplyPreview{
		ID:          m.ID.String(),
		SenderID:    m.SenderID.String(),
		MessageType: m.MessageType,
		Content:     m.Content,
		Deleted:     m.DeletedAt != nil,
	}
	if m.MessageType == MessageTypeText && len(m.Content) > 0 {
		var body textContent
		if json.Unmarshal(m.Content, &body) == nil {
			p.Text = body.Text
		}
	}
	return p
}

// reactionsResponse aggregates raw reaction rows by emoji, preserving first-seen
// order and marking the viewer's own reactions.
func reactionsResponse(reactions []Reaction, viewer uuid.UUID) []ReactionResponse {
	out := make([]ReactionResponse, 0)
	if len(reactions) == 0 {
		return out
	}
	index := make(map[string]int, len(reactions))
	for _, r := range reactions {
		i, ok := index[r.Emoji]
		if !ok {
			index[r.Emoji] = len(out)
			out = append(out, ReactionResponse{Emoji: r.Emoji, Users: []string{}})
			i = len(out) - 1
		}
		out[i].Count++
		out[i].Users = append(out[i].Users, r.UserID.String())
		if r.UserID == viewer {
			out[i].Mine = true
		}
	}
	return out
}

// ConversationResponse is one conversation in a list or a single fetch. For
// direct conversations `peer` is populated; for groups `title`, `avatar_url`,
// and `member_count` are, and `peer` is null.
type ConversationResponse struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Peer        *UserBriefResponse `json:"peer"`
	Title       *string            `json:"title"`
	AvatarURL   *string            `json:"avatar_url"`
	MemberCount int64              `json:"member_count"`
	Role        string             `json:"role"`
	LastMessage *MessageResponse   `json:"last_message"`
	UnreadCount int64              `json:"unread_count"`
	CreatedAt   string             `json:"created_at"`
}

func toConversationResponse(s ConversationSummary, online bool) ConversationResponse {
	resp := ConversationResponse{
		ID:          s.ID.String(),
		Kind:        s.Kind,
		Title:       s.Title,
		AvatarURL:   s.AvatarURL,
		MemberCount: s.MemberCount,
		Role:        s.Role,
		UnreadCount: s.Unread,
		CreatedAt:   s.CreatedAt.UTC().Format(timeFormat),
	}
	if s.Kind == "direct" {
		peer := toUserBrief(s.Peer, online)
		resp.Peer = &peer
	}
	if s.LastMessage != nil {
		m := ToMessageResponse(*s.LastMessage)
		resp.LastMessage = &m
	}
	return resp
}

// MemberResponse is one group member joined with their public profile.
type MemberResponse struct {
	ID            string  `json:"id"`
	Role          string  `json:"role"`
	DisplayName   string  `json:"display_name"`
	Username      *string `json:"username"`
	AvatarURL     *string `json:"avatar_url"`
	StatusMessage *string `json:"status_message"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"last_seen_at"`
	JoinedAt      string  `json:"joined_at"`
}

func toMemberResponse(m MemberDetail, online bool) MemberResponse {
	resp := MemberResponse{
		ID:            m.UserID.String(),
		Role:          m.Role,
		DisplayName:   m.DisplayName,
		Username:      m.Username,
		AvatarURL:     m.AvatarURL,
		StatusMessage: m.StatusMessage,
		Online:        online,
		JoinedAt:      m.JoinedAt.UTC().Format(timeFormat),
	}
	if m.LastSeenAt != nil {
		s := m.LastSeenAt.UTC().Format(timeFormat)
		resp.LastSeenAt = &s
	}
	return resp
}

// --- request payloads -------------------------------------------------------

// openConversationInput is the body for POST /conversations.
type openConversationInput struct {
	UserID string `json:"user_id"`
}

// Validate normalizes the input and returns per-field errors.
func (in *openConversationInput) Validate() map[string]string {
	in.UserID = trimSpace(in.UserID)
	fields := map[string]string{}
	if in.UserID == "" {
		fields["user_id"] = "A user id is required."
		return fields
	}
	if _, err := parseUUID(in.UserID); err != nil {
		fields["user_id"] = "That is not a valid user id."
	}
	return fields
}

// SendInput is the body for POST /conversations/:id/messages and the payload of
// a realtime `message.send` event. It is exported so the realtime transport can
// construct one.
type SendInput struct {
	ClientMsgID string          `json:"client_msg_id"`
	MessageType string          `json:"message_type"`
	Content     json.RawMessage `json:"content"`
	// ReplyToID optionally quotes an earlier message in the same conversation.
	ReplyToID string `json:"reply_to_id"`
}

// Validate checks the closed message_type set and the content shape, returning
// per-field errors for a 422. `message_type` and `content` are normalized in
// place so callers persist exactly what was validated.
func (in *SendInput) Validate() map[string]string {
	fields := map[string]string{}

	in.ClientMsgID = trimSpace(in.ClientMsgID)
	if in.ClientMsgID == "" {
		fields["client_msg_id"] = "A client message id is required for retries."
	} else if len(in.ClientMsgID) > 64 {
		fields["client_msg_id"] = "Client message id must be at most 64 characters."
	}

	in.ReplyToID = trimSpace(in.ReplyToID)
	if in.ReplyToID != "" {
		if _, err := parseUUID(in.ReplyToID); err != nil {
			fields["reply_to_id"] = "That is not a valid message id."
		}
	}

	if in.MessageType == "" {
		in.MessageType = MessageTypeText
	}
	if len(in.Content) > maxContentByte {
		return map[string]string{"content": "Message is too large."}
	}

	normalized, contentFields := validateContent(in.MessageType, in.Content)
	if len(contentFields) > 0 {
		for k, v := range contentFields {
			fields[k] = v
		}
	} else {
		in.Content = normalized
	}
	return fields
}

type textContent struct {
	Text string `json:"text"`
}

// imageContent and audioContent carry a media reference. `duration` (seconds)
// is optional client-reported metadata for voice notes.
type imageContent struct {
	URL string `json:"url"`
}

type audioContent struct {
	URL      string `json:"url"`
	Duration int    `json:"duration,omitempty"`
}

type videoContent struct {
	URL string `json:"url"`
}

// fileContent carries a media reference plus the original file's display name
// and size, so the recipient can render a useful attachment card.
type fileContent struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// media URL bounds: relative paths served by this API are expected, but any
// absolute URL is accepted for future object storage.
const maxMediaURLLength = 2048

// validateMediaURL rejects empty, oversized, or control-character-laden urls.
func validateMediaURL(raw string) (string, bool) {
	raw = trimSpace(raw)
	if raw == "" || len(raw) > maxMediaURLLength {
		return "", false
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return raw, true
}

// validateContent enforces a closed shape per message type and returns the
// canonical content to persist. Arbitrary JSON is rejected, and the text bound
// is counted in runes so emoji are not truncated mid-codepoint.
func validateContent(messageType string, content json.RawMessage) (json.RawMessage, map[string]string) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, map[string]string{"content": "Message content is required."}
	}

	decode := func(dst any) bool {
		dec := json.NewDecoder(bytes.NewReader(content))
		dec.DisallowUnknownFields()
		return dec.Decode(dst) == nil
	}

	switch messageType {
	case MessageTypeText:
		var body textContent
		if !decode(&body) {
			return nil, map[string]string{"content": "Text messages require a text field."}
		}
		body.Text = trimSpace(body.Text)
		switch {
		case body.Text == "":
			return nil, map[string]string{"content": "Message cannot be empty."}
		case utf8.RuneCountInString(body.Text) > maxTextLength:
			return nil, map[string]string{"content": "Message must be at most 4000 characters."}
		}
		return mustMarshal(body), nil

	case MessageTypeImage:
		var body imageContent
		if !decode(&body) {
			return nil, map[string]string{"content": "Image messages require a url field."}
		}
		url, ok := validateMediaURL(body.URL)
		if !ok {
			return nil, map[string]string{"content": "A valid image url is required."}
		}
		body.URL = url
		return mustMarshal(body), nil

	case MessageTypeAudio:
		var body audioContent
		if !decode(&body) {
			return nil, map[string]string{"content": "Audio messages require a url field."}
		}
		url, ok := validateMediaURL(body.URL)
		if !ok {
			return nil, map[string]string{"content": "A valid audio url is required."}
		}
		body.URL = url
		if body.Duration < 0 {
			body.Duration = 0
		}
		return mustMarshal(body), nil

	case MessageTypeFile:
		var body fileContent
		if !decode(&body) {
			return nil, map[string]string{"content": "File messages require a url field."}
		}
		url, ok := validateMediaURL(body.URL)
		if !ok {
			return nil, map[string]string{"content": "A valid file url is required."}
		}
		body.URL = url
		body.Name = trimSpace(body.Name)
		if utf8.RuneCountInString(body.Name) > maxFileNameLength {
			return nil, map[string]string{"content": "File name is too long."}
		}
		if body.Size < 0 {
			body.Size = 0
		}
		return mustMarshal(body), nil

	case MessageTypeVideo:
		var body videoContent
		if !decode(&body) {
			return nil, map[string]string{"content": "Video messages require a url field."}
		}
		url, ok := validateMediaURL(body.URL)
		if !ok {
			return nil, map[string]string{"content": "A valid video url is required."}
		}
		body.URL = url
		return mustMarshal(body), nil

	case MessageTypeSystem:
		return nil, map[string]string{"message_type": "System messages are server-generated."}
	default:
		return nil, map[string]string{"message_type": "Unsupported message type."}
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// markReadInput is the body for POST /conversations/:id/read.
type markReadInput struct {
	SequenceNo *int64 `json:"sequence_no"`
}

// Validate returns per-field errors for the read position.
func (in *markReadInput) Validate() map[string]string {
	fields := map[string]string{}
	if in.SequenceNo == nil {
		fields["sequence_no"] = "A sequence number is required."
	} else if *in.SequenceNo < 0 {
		fields["sequence_no"] = "Sequence number cannot be negative."
	}
	return fields
}

// --- advanced messaging (Phase 4) payloads ----------------------------------

// maxEmojiLength bounds a reaction; some emoji are multi-codepoint sequences.
const maxEmojiLength = 16

// maxSearchQueryLength bounds an in-chat search term.
const maxSearchQueryLength = 100

// editMessageInput is the body for PATCH /conversations/:id/messages/:messageId.
type editMessageInput struct {
	Text string `json:"text"`
}

// Validate normalizes the new body and returns per-field errors.
func (in *editMessageInput) Validate() map[string]string {
	in.Text = trimSpace(in.Text)
	switch {
	case in.Text == "":
		return map[string]string{"text": "Message cannot be empty."}
	case utf8.RuneCountInString(in.Text) > maxTextLength:
		return map[string]string{"text": "Message must be at most 4000 characters."}
	}
	return nil
}

// forwardInput is the body for POST /conversations/:id/messages/forward, where
// :id is the destination conversation.
type forwardInput struct {
	SourceConversationID string `json:"source_conversation_id"`
	MessageID            string `json:"message_id"`
}

// Validate normalizes the ids and returns per-field errors.
func (in *forwardInput) Validate() map[string]string {
	fields := map[string]string{}
	in.SourceConversationID = trimSpace(in.SourceConversationID)
	in.MessageID = trimSpace(in.MessageID)
	if _, err := parseUUID(in.SourceConversationID); err != nil {
		fields["source_conversation_id"] = "That is not a valid conversation id."
	}
	if _, err := parseUUID(in.MessageID); err != nil {
		fields["message_id"] = "That is not a valid message id."
	}
	return fields
}

// reactionInput is the body for adding a reaction.
type reactionInput struct {
	Emoji string `json:"emoji"`
}

// Validate requires a short, non-empty emoji.
func (in *reactionInput) Validate() map[string]string {
	in.Emoji = trimSpace(in.Emoji)
	switch {
	case in.Emoji == "":
		return map[string]string{"emoji": "An emoji is required."}
	case utf8.RuneCountInString(in.Emoji) > maxEmojiLength:
		return map[string]string{"emoji": "That reaction is too long."}
	}
	return nil
}

// --- group payloads ---------------------------------------------------------

// Group bounds.
const (
	maxGroupTitleLength = 64
	maxGroupMembers     = 256
	maxMemberBatch      = 100
)

// createGroupInput is the body for POST /conversations/group.
type createGroupInput struct {
	Title     string   `json:"title"`
	MemberIDs []string `json:"member_ids"`
}

// Validate normalizes the input and returns per-field errors.
func (in *createGroupInput) Validate() map[string]string {
	fields := map[string]string{}
	in.Title = trimSpace(in.Title)
	switch {
	case in.Title == "":
		fields["title"] = "A group name is required."
	case utf8.RuneCountInString(in.Title) > maxGroupTitleLength:
		fields["title"] = "Group name must be at most 64 characters."
	}
	if len(in.MemberIDs) > maxGroupMembers {
		fields["member_ids"] = "Too many members."
	}
	for _, id := range in.MemberIDs {
		if _, err := parseUUID(id); err != nil {
			fields["member_ids"] = "One or more member ids are invalid."
			break
		}
	}
	return fields
}

// updateGroupInput is the body for PATCH /conversations/:id.
type updateGroupInput struct {
	Title     *string `json:"title"`
	AvatarURL *string `json:"avatar_url"`
}

// Validate normalizes the input and returns per-field errors.
func (in *updateGroupInput) Validate() map[string]string {
	fields := map[string]string{}
	if in.Title == nil && in.AvatarURL == nil {
		fields["title"] = "Provide a title or avatar_url to update."
		return fields
	}
	if in.Title != nil {
		v := trimSpace(*in.Title)
		switch {
		case v == "":
			fields["title"] = "Group name cannot be empty."
		case utf8.RuneCountInString(v) > maxGroupTitleLength:
			fields["title"] = "Group name must be at most 64 characters."
		default:
			in.Title = &v
		}
	}
	if in.AvatarURL != nil {
		v, ok := validateMediaURL(*in.AvatarURL)
		if !ok {
			fields["avatar_url"] = "A valid avatar url is required."
		} else {
			in.AvatarURL = &v
		}
	}
	return fields
}

// addMembersInput is the body for POST /conversations/:id/members.
type addMembersInput struct {
	UserIDs []string `json:"user_ids"`
}

// Validate checks the batch and returns per-field errors.
func (in *addMembersInput) Validate() map[string]string {
	fields := map[string]string{}
	switch {
	case len(in.UserIDs) == 0:
		fields["user_ids"] = "At least one user id is required."
	case len(in.UserIDs) > maxMemberBatch:
		fields["user_ids"] = "Too many users in one request."
	}
	for _, id := range in.UserIDs {
		if _, err := parseUUID(id); err != nil {
			fields["user_ids"] = "One or more user ids are invalid."
			break
		}
	}
	return fields
}

// setRoleInput is the body for POST /conversations/:id/members/:userId/role.
type setRoleInput struct {
	Role string `json:"role"`
}

// Validate checks the role against the assignable set.
func (in *setRoleInput) Validate() map[string]string {
	fields := map[string]string{}
	in.Role = trimSpace(in.Role)
	if in.Role != RoleAdmin && in.Role != RoleMember {
		fields["role"] = "Role must be admin or member."
	}
	return fields
}
