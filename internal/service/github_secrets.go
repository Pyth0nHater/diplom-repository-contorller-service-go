package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/box"
)

// SetGitHubRepoSecrets sets GitHub Actions secrets on the given repository.
// repoURL may be "https://github.com/owner/repo" or "https://github.com/owner/repo.git".
func SetGitHubRepoSecrets(ctx context.Context, accessToken, repoURL string, secrets map[string]string) error {
	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return err
	}

	pubKey, keyID, err := getRepoPublicKey(ctx, accessToken, owner, repo)
	if err != nil {
		return fmt.Errorf("get repo public key: %w", err)
	}

	for name, value := range secrets {
		encrypted, err := encryptSecret(pubKey, value)
		if err != nil {
			return fmt.Errorf("encrypt secret %s: %w", name, err)
		}
		if err := putRepoSecret(ctx, accessToken, owner, repo, name, keyID, encrypted); err != nil {
			return fmt.Errorf("set secret %s: %w", name, err)
		}
	}

	return nil
}

func parseGitHubRepoURL(repoURL string) (owner, repo string, err error) {
	u := strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
	u = strings.TrimPrefix(u, "https://github.com/")
	u = strings.TrimPrefix(u, "http://github.com/")
	parts := strings.SplitN(u, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse github owner/repo from %q", repoURL)
	}
	return parts[0], parts[1], nil
}

type repoPublicKeyResponse struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

func getRepoPublicKey(ctx context.Context, accessToken, owner, repo string) (pubKey [32]byte, keyID string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/public-key", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return pubKey, "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return pubKey, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return pubKey, "", fmt.Errorf("GitHub API %s: %s", resp.Status, body)
	}

	var result repoPublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return pubKey, "", err
	}

	keyBytes, err := base64.StdEncoding.DecodeString(result.Key)
	if err != nil {
		return pubKey, "", fmt.Errorf("decode public key: %w", err)
	}
	if len(keyBytes) != 32 {
		return pubKey, "", fmt.Errorf("unexpected public key length: %d", len(keyBytes))
	}
	copy(pubKey[:], keyBytes)

	return pubKey, result.KeyID, nil
}

// encryptSecret encrypts plaintext for a GitHub Actions secret using NaCl sealed box
// with a blake2b-256 nonce (matching libsodium's crypto_box_seal).
func encryptSecret(recipientPub [32]byte, plaintext string) (string, error) {
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}

	// nonce = blake2b(ephemeralPub || recipientPub) with 24-byte output (matches libsodium crypto_box_seal)
	h, err := blake2b.New(24, nil)
	if err != nil {
		return "", err
	}
	h.Write(ephemeralPub[:])
	h.Write(recipientPub[:])

	var nonce [24]byte
	copy(nonce[:], h.Sum(nil))

	// encrypted = box.Seal(plaintext, nonce, recipientPub, ephemeralPriv)
	encrypted := box.Seal(nil, []byte(plaintext), &nonce, &recipientPub, ephemeralPriv)

	// result = ephemeralPub || encrypted
	result := make([]byte, 32+len(encrypted))
	copy(result[:32], ephemeralPub[:])
	copy(result[32:], encrypted)

	return base64.StdEncoding.EncodeToString(result), nil
}

func putRepoSecret(ctx context.Context, accessToken, owner, repo, secretName, keyID, encryptedValue string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/%s", owner, repo, secretName)

	payload, err := json.Marshal(map[string]string{
		"encrypted_value": encryptedValue,
		"key_id":          keyID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %s: %s", resp.Status, body)
	}

	return nil
}
