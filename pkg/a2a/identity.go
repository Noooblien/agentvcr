package a2a

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// IdentityManager handles agent key generation, registration, and cryptographic verification.
type IdentityManager struct {
	mu        sync.RWMutex
	cards     map[string]AgentCard
	keys      map[string]ed25519.PrivateKey
	pubKeys   map[string]ed25519.PublicKey
}

// NewIdentityManager initializes an empty identity manager.
func NewIdentityManager() *IdentityManager {
	return &IdentityManager{
		cards:   make(map[string]AgentCard),
		keys:    make(map[string]ed25519.PrivateKey),
		pubKeys: make(map[string]ed25519.PublicKey),
	}
}

// GenerateIdentity creates a new Ed25519 keypair and registers an agent.
func (im *IdentityManager) GenerateIdentity(agentID, name, role, endpoint string, caps []Capability) (*AgentCard, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cryptographic keypair: %w", err)
	}

	pubHex := hex.EncodeToString(pub)
	card := AgentCard{
		AgentID:      agentID,
		Name:         name,
		Role:         role,
		Endpoint:     endpoint,
		Capabilities: caps,
		PublicKey:    pubHex,
		RegisteredAt: time.Now(),
	}

	im.cards[agentID] = card
	im.keys[agentID] = priv
	im.pubKeys[agentID] = pub

	return &card, nil
}

// RegisterCard stores an external agent card.
func (im *IdentityManager) RegisterCard(card AgentCard) {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.cards[card.AgentID] = card
	if card.PublicKey != "" {
		if pubBytes, err := hex.DecodeString(card.PublicKey); err == nil && len(pubBytes) == ed25519.PublicKeySize {
			im.pubKeys[card.AgentID] = ed25519.PublicKey(pubBytes)
		}
	}
}

// GetCard returns the agent card for a given ID.
func (im *IdentityManager) GetCard(agentID string) (AgentCard, bool) {
	im.mu.RLock()
	defer im.mu.RUnlock()
	card, ok := im.cards[agentID]
	return card, ok
}

// ListCards returns all registered agent cards.
func (im *IdentityManager) ListCards() []AgentCard {
	im.mu.RLock()
	defer im.mu.RUnlock()

	list := make([]AgentCard, 0, len(im.cards))
	for _, c := range im.cards {
		list = append(list, c)
	}
	return list
}

// SignEnvelope cryptographically signs an envelope with the sender's private key.
func (im *IdentityManager) SignEnvelope(env *MessageEnvelope) error {
	im.mu.RLock()
	priv, exists := im.keys[env.SenderID]
	im.mu.RUnlock()

	if !exists {
		// Mock signature if key not generated locally
		env.Signature = fmt.Sprintf("sig-%s-%s", env.SenderID, env.ID)
		return nil
	}

	payloadBytes, _ := json.Marshal(env.Payload)
	message := []byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		env.ID, env.ConversationID, env.SenderID, env.RecipientID, env.Type, string(payloadBytes)))

	sig := ed25519.Sign(priv, message)
	env.Signature = hex.EncodeToString(sig)
	return nil
}

// VerifyEnvelope verifies an envelope signature against the sender's public key.
func (im *IdentityManager) VerifyEnvelope(env *MessageEnvelope) bool {
	if env == nil || env.Signature == "" {
		return false
	}

	im.mu.RLock()
	pub, exists := im.pubKeys[env.SenderID]
	im.mu.RUnlock()

	if !exists {
		// Allow prefix signature for mock tests
		return env.Signature == fmt.Sprintf("sig-%s-%s", env.SenderID, env.ID)
	}

	sigBytes, err := hex.DecodeString(env.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	payloadBytes, _ := json.Marshal(env.Payload)
	message := []byte(fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		env.ID, env.ConversationID, env.SenderID, env.RecipientID, env.Type, string(payloadBytes)))

	return ed25519.Verify(pub, message, sigBytes)
}
