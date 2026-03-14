package protocol

import (
	"encoding/json"
)

// Beat 对应 Beats.json 结构
type Beat struct {
	Op      string      `json:"op"`
	Payload BeatPayload `json:"payload"`
}

type BeatPayload struct {
	Version   []int  `json:"version"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
	Delay     int    `json:"delay"`
}

// Message 对应 Messages.json 结构
type Message struct {
	Op      string         `json:"op"`
	Payload MessagePayload `json:"payload"`
}

type MessagePayload struct {
	ID        string `json:"id"`
	Form      string `json:"form"`
	To        string `json:"to"`
	Type      string `json:"type"` // Text/Image/System
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Note 对应 Notes.json 结构
type Note struct {
	Op      string      `json:"op"`
	Payload NotePayload `json:"payload"`
}

type NotePayload struct {
	Type      string `json:"type"` // ack/notify/error
	Code      string `json:"code"`
	Timestamp int64  `json:"timestamp"`
}

// Auth 对应 login.json 结构
type Auth struct {
	Op      string      `json:"op"`
	Payload AuthPayload `json:"payload"`
}

type AuthPayload struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// ToJSON Beat
func (b *Beat) ToJSON() ([]byte, error) {
	return json.Marshal(b)
}

// BeatFromJSON Beat
func BeatFromJSON(data []byte) (*Beat, error) {
	var beat Beat
	err := json.Unmarshal(data, &beat)
	if err != nil {
		return nil, err
	}
	return &beat, nil
}

// ToJSON Message
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// MessageFromJSON Message
func MessageFromJSON(data []byte) (*Message, error) {
	var message Message
	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// ToJSON Note
func (n *Note) ToJSON() ([]byte, error) {
	return json.Marshal(n)
}

// NoteFromJSON Note
func NoteFromJSON(data []byte) (*Note, error) {
	var note Note
	err := json.Unmarshal(data, &note)
	if err != nil {
		return nil, err
	}
	return &note, nil
}

// ToJSON Auth
func (a *Auth) ToJSON() ([]byte, error) {
	return json.Marshal(a)
}

// AuthFromJSON Auth
func AuthFromJSON(data []byte) (*Auth, error) {
	var auth Auth
	err := json.Unmarshal(data, &auth)
	if err != nil {
		return nil, err
	}
	return &auth, nil
}
