package oauth

import (
	"encoding/base64"
	"testing"
)

func createTestJWT(header, payload string) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(header))
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))
	s := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return h + "." + p + "." + s
}

func TestExtractJWTIdentity(t *testing.T) {
	jwtWithEmail := createTestJWT(`{"alg":"none"}`, `{"email":"jane.doe@example.com","sub":"user_123"}`)
	jwtWithSubOnly := createTestJWT(`{"alg":"none"}`, `{"sub":"user_456"}`)
	jwtWithoutIdentity := createTestJWT(`{"alg":"none"}`, `{"aud":"my-client","exp":1234567890}`)

	tests := []struct {
		name  string
		token string
		want  string
	}{
		{
			name:  "email and sub returns email",
			token: jwtWithEmail,
			want:  "jane.doe@example.com",
		},
		{
			name:  "sub only returns sub",
			token: jwtWithSubOnly,
			want:  "user_456",
		},
		{
			name:  "no email or sub returns empty",
			token: jwtWithoutIdentity,
			want:  "",
		},
		{
			name:  "opaque token returns empty",
			token: "opaque-random-token-value-123456",
			want:  "",
		},
		{
			name:  "two segments returns empty",
			token: "header.payload",
			want:  "",
		},
		{
			name:  "four segments returns empty",
			token: "header.payload.sig.extra",
			want:  "",
		},
		{
			name:  "invalid payload json returns empty",
			token: "header." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".sig",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractJWTIdentity(tt.token)
			if got != tt.want {
				t.Errorf("ExtractJWTIdentity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractIdentityHint(t *testing.T) {
	idTokenJWT := createTestJWT(`{"alg":"none"}`, `{"email":"idtoken@example.com"}`)
	accessTokenJWT := createTestJWT(`{"alg":"none"}`, `{"email":"accesstoken@example.com"}`)

	t.Run("prefers id_token when both present", func(t *testing.T) {
		got := ExtractIdentityHint(idTokenJWT, accessTokenJWT)
		if got != "idtoken@example.com" {
			t.Errorf("ExtractIdentityHint() = %q, want %q", got, "idtoken@example.com")
		}
	})

	t.Run("falls back to access_token if id_token has no identity", func(t *testing.T) {
		emptyIDToken := createTestJWT(`{"alg":"none"}`, `{"aud":"client"}`)
		got := ExtractIdentityHint(emptyIDToken, accessTokenJWT)
		if got != "accesstoken@example.com" {
			t.Errorf("ExtractIdentityHint() = %q, want %q", got, "accesstoken@example.com")
		}
	})

	t.Run("returns empty if neither has identity", func(t *testing.T) {
		got := ExtractIdentityHint("opaque1", "opaque2")
		if got != "" {
			t.Errorf("ExtractIdentityHint() = %q, want empty", got)
		}
	})
}
