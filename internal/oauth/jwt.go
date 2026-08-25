package oauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// ExtractJWTIdentity attempts to parse an unverified JWT payload and returns email or sub claims.
// Returns empty string if the token is not a valid 3-segment JWT or contains no email/sub claim.
func ExtractJWTIdentity(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	// Handle raw URL encoding or standard URL encoding with/without padding
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return ""
		}
	}
	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	if claims.Email != "" {
		return claims.Email
	}
	return claims.Sub
}

// ExtractIdentityHint derives an identity hint from an id_token (preferred email/sub) or access_token (email/sub).
func ExtractIdentityHint(idToken, accessToken string) string {
	if idToken != "" {
		if hint := ExtractJWTIdentity(idToken); hint != "" {
			return hint
		}
	}
	if accessToken != "" {
		if hint := ExtractJWTIdentity(accessToken); hint != "" {
			return hint
		}
	}
	return ""
}
