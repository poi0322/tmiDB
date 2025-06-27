package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// CreateAdminUser는 관리자 사용자를 생성합니다 (초기 설정용)
// 이 함수는 이제 CreateOrgAndAdminUser로 대체될 수 있지만, 이전 로직과의 호환성을 위해 남겨둘 수 있습니다.
func CreateAdminUser(username, password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	_, err = DB.Exec("INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'admin')", username, string(hashedPassword))
	if err != nil {
		return "", err
	}
	// TODO: 이 사용자를 위한 토큰 생성 로직 추가 필요
	return "temp_token", nil // 임시 토큰 반환
}

// AuthenticateUser는 사용자를 인증하고 성공 시 사용자 ID, 조직 ID, 역할을 반환합니다.
func AuthenticateUser(username, password string) (userID, orgID, role string, err error) {
	var storedHash string
	err = DB.QueryRow("SELECT user_id, org_id, password_hash, role FROM users WHERE username = $1 AND is_active = TRUE", username).Scan(&userID, &orgID, &storedHash, &role)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", fmt.Errorf("user not found or not active")
		}
		return "", "", "", err
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err != nil {
		return "", "", "", fmt.Errorf("invalid password")
	}

	return userID, orgID, role, nil
}

// CreateOrgAndAdminUser는 새 조직과 해당 조직의 관리자를 원자적으로 생성합니다.
func CreateOrgAndAdminUser(orgName, username, password string) (string, error) {
	tx, err := DB.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback on error

	// 1. 조직 생성 (이미 존재하면 ID를 가져옴)
	var orgID string
	err = tx.QueryRow(`SELECT org_id FROM organizations WHERE name = $1`, orgName).Scan(&orgID)
	if err != nil {
		if err == sql.ErrNoRows {
			// 조직이 없으면 새로 생성
			err = tx.QueryRow(`INSERT INTO organizations (name) VALUES ($1) RETURNING org_id`, orgName).Scan(&orgID)
			if err != nil {
				return "", fmt.Errorf("failed to create organization: %w", err)
			}
		} else {
			// 다른 데이터베이스 오류
			return "", fmt.Errorf("failed to check for organization: %w", err)
		}
	}

	// 2. 관리자 사용자 생성 (이미 존재하면 넘어감)
	var existingUser string
	err = tx.QueryRow(`SELECT user_id FROM users WHERE org_id = $1 AND username = $2`, orgID, username).Scan(&existingUser)
	if err != nil {
		if err == sql.ErrNoRows {
			// 사용자가 없으면 새로 생성
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return "", fmt.Errorf("failed to hash password: %w", err)
			}
			_, err = tx.Exec(`
				INSERT INTO users (org_id, username, password_hash, role, is_active)
				VALUES ($1, $2, $3, 'admin', TRUE)
			`, orgID, username, string(hashedPassword))
			if err != nil {
				return "", fmt.Errorf("failed to create admin user: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to check for admin user: %w", err)
		}
	}

	// 3. 관리자용 API 토큰 생성
	// 참고: 이 부분은 멱등성이 없어서 재실행 시마다 새 토큰을 만들 수 있습니다.
	// 초기 설정에서는 문제가 되지 않습니다.
	accessToken, err := GenerateAndSaveAuthToken(tx, orgID, "Initial admin token", true)
	if err != nil {
		return "", fmt.Errorf("failed to create admin access token: %w", err)
	}

	// 4. 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return accessToken, nil
}

// GenerateAndSaveAuthToken는 새로운 API 토큰을 생성, 암호화, 저장합니다.
func GenerateAndSaveAuthToken(db DBTX, orgID, description string, isAdmin bool) (string, error) {
	// 1. 원본 토큰 생성 (32 bytes -> 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("could not generate token: %w", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// 2. 토큰 암호화
	encryptedToken, err := EncryptToken(tokenString)
	if err != nil {
		return "", fmt.Errorf("could not encrypt token: %w", err)
	}

	// 3. 권한 설정
	var permissions string
	if isAdmin {
		permissions = `{"admin": true}`
	} else {
		permissions = `{"read": ["*"], "write": []}`
	}

	// 4. 데이터베이스에 저장
	_, err = db.Exec(`
		INSERT INTO auth_tokens (org_id, encrypted_token, description, permissions, is_admin, is_active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
	`, orgID, encryptedToken, description, permissions, isAdmin)
	if err != nil {
		return "", fmt.Errorf("could not save token to database: %w", err)
	}

	return tokenString, nil
}

// AuthenticateToken은 제공된 토큰이 유효한지 확인하고 권한을 반환합니다.
func AuthenticateToken(tokenString string) (bool, map[string]interface{}, error) {
	// 토큰을 해싱하여 저장된 값과 비교하는 로직 필요
	// 현재는 임시로 true를 반환
	// TODO: 실제 토큰 인증 로직 구현
	var storedHash string
	var permissions map[string]interface{}
	// SELECT token_hash, permissions FROM auth_tokens WHERE ...
	err := DB.QueryRow("SELECT ...").Scan(&storedHash, &permissions)
	if err != nil {
		return false, nil, err
	}
	// ... 해시 비교 ...
	return true, permissions, nil
}

// GetAuthTokens는 특정 조직의 모든 인증 토큰을 조회합니다.
func GetAuthTokens(orgID string) ([]AuthToken, error) {
	rows, err := DB.Query(`
		SELECT token_id, encrypted_token, description, permissions, is_admin, is_active, expires_at, created_at
		FROM auth_tokens 
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []AuthToken
	for rows.Next() {
		var token AuthToken
		if err := rows.Scan(
			&token.TokenID,
			&token.EncryptedToken,
			&token.Description,
			&token.Permissions,
			&token.IsAdmin,
			&token.IsActive,
			&token.ExpiresAt,
			&token.CreatedAt,
		); err != nil {
			// Log the error and continue with the next row
			fmt.Printf("Error scanning token row: %v\n", err)
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// DeleteAuthToken은 특정 조직에서 토큰 ID를 기반으로 토큰을 삭제합니다.
func DeleteAuthToken(tokenID, orgID string) error {
	_, err := DB.Exec("DELETE FROM auth_tokens WHERE token_id = $1 AND org_id = $2", tokenID, orgID)
	return err
}

type AuthToken struct {
	TokenID        string         `json:"token_id"`
	UserID         string         `json:"user_id"`
	OrgID          string         `json:"org_id"`
	EncryptedToken string         `json:"-"` // JSON에 포함되지 않음
	DecryptedToken string         `json:"token,omitempty"`
	Description    sql.NullString `json:"description"`
	Permissions    sql.NullString `json:"permissions"`
	IsAdmin        bool           `json:"is_admin"`
	IsActive       bool           `json:"is_active"`
	ExpiresAt      sql.NullTime   `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Legacy - Hashing related functions, might be removed later
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// User represents a user in the system.
type User struct {
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Username  string    `json:"username"`
	Password  string    `json:"password,omitempty"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetUsers는 특정 조직의 모든 사용자를 조회합니다.
func GetUsers(orgID string) ([]User, error) {
	rows, err := DB.Query("SELECT user_id, org_id, username, role, is_active, created_at, updated_at FROM users WHERE org_id = $1 ORDER BY created_at DESC", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UserID, &u.OrgID, &u.Username, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// CreateUser는 특정 조직에 새 사용자를 생성합니다.
func CreateUser(user User) (*User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	err = DB.QueryRow(
		"INSERT INTO users (org_id, username, password_hash, role, is_active) VALUES ($1, $2, $3, $4, $5) RETURNING user_id, created_at, updated_at",
		user.OrgID, user.Username, string(hashedPassword), user.Role, user.IsActive,
	).Scan(&user.UserID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, err
	}
	user.Password = "" // Don't send password back
	return &user, nil
}

// UpdateUser는 특정 조직에서 사용자를 업데이트합니다.
func UpdateUser(user User) (*User, error) {
	// 비밀번호가 제공된 경우 해시하여 업데이트합니다.
	if user.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		_, err = DB.Exec(
			"UPDATE users SET role = $1, is_active = $2, password_hash = $3, updated_at = NOW() WHERE user_id = $4 AND org_id = $5",
			user.Role, user.IsActive, string(hashedPassword), user.UserID, user.OrgID,
		)
		if err != nil {
			return nil, err
		}
	} else {
		// 비밀번호 변경이 없는 경우
		_, err := DB.Exec(
			"UPDATE users SET role = $1, is_active = $2, updated_at = NOW() WHERE user_id = $3 AND org_id = $4",
			user.Role, user.IsActive, user.UserID, user.OrgID,
		)
		if err != nil {
			return nil, err
		}
	}

	// 업데이트된 사용자 정보를 다시 조회하여 반환합니다.
	var updatedUser User
	err := DB.QueryRow("SELECT user_id, org_id, username, role, is_active, created_at, updated_at FROM users WHERE user_id = $1", user.UserID).Scan(
		&updatedUser.UserID, &updatedUser.OrgID, &updatedUser.Username, &updatedUser.Role, &updatedUser.IsActive, &updatedUser.CreatedAt, &updatedUser.UpdatedAt,
	)
	if err != nil {
		// 조회 실패 시에도 최소한의 정보로 응답할 수 있도록 user 객체를 반환할 수 있지만,
		// 일관성을 위해 오류를 반환합니다.
		return nil, fmt.Errorf("failed to retrieve updated user data: %w", err)
	}

	return &updatedUser, nil
}

// DeleteUser는 특정 조직에서 사용자를 삭제합니다.
func DeleteUser(id, orgID string) error {
	_, err := DB.Exec("DELETE FROM users WHERE user_id = $1 AND org_id = $2", id, orgID)
	return err
}

// === User Access Token Management ===

// CreateUserToken은 특정 사용자를 위한 새 액세스 토큰을 생성하고 저장합니다.
// 원본 토큰은 반환되고, 해시된 값은 DB에 저장됩니다.
func CreateUserToken(userID, orgID, description string) (string, *AuthToken, error) {
	// 1. 원본 토큰 생성
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("could not generate token: %w", err)
	}
	tokenString := hex.EncodeToString(tokenBytes)

	// 2. 토큰 해시
	tokenHash := hashToken(tokenString)

	// 3. 데이터베이스에 저장
	var createdToken AuthToken
	err := DB.QueryRow(`
		INSERT INTO user_access_tokens (user_id, org_id, token_hash, description, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		RETURNING token_id, user_id, org_id, description, is_active, created_at
	`, userID, orgID, tokenHash, description).Scan(
		&createdToken.TokenID, &createdToken.UserID, &createdToken.OrgID, &createdToken.Description, &createdToken.IsActive, &createdToken.CreatedAt,
	)

	if err != nil {
		return "", nil, fmt.Errorf("could not save token to database: %w", err)
	}

	return tokenString, &createdToken, nil
}

// GetUserTokens는 특정 사용자의 모든 활성 액세스 토큰을 조회합니다.
func GetUserTokens(userID, orgID string) ([]AuthToken, error) {
	rows, err := DB.Query(`
		SELECT token_id, user_id, org_id, description, is_active, created_at 
		FROM user_access_tokens 
		WHERE user_id = $1 AND org_id = $2
		ORDER BY created_at DESC
	`, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTokens(rows)
}

// GetAllUserTokens는 특정 조직의 모든 사용자의 활성 액세스 토큰을 조회합니다. (관리자용)
func GetAllUserTokens(orgID string) ([]AuthToken, error) {
	rows, err := DB.Query(`
		SELECT token_id, user_id, org_id, description, is_active, created_at 
		FROM user_access_tokens 
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTokens(rows)
}

// DeleteUserToken은 특정 사용자가 자신의 액세스 토큰을 삭제합니다.
func DeleteUserToken(tokenID, userID, orgID string) error {
	res, err := DB.Exec("DELETE FROM user_access_tokens WHERE token_id = $1 AND user_id = $2 AND org_id = $3", tokenID, userID, orgID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("token not found or you do not have permission to delete it")
	}
	return nil
}

// DeleteUserTokenAsAdmin은 관리자가 조직 내의 모든 액세스 토큰을 삭제합니다.
func DeleteUserTokenAsAdmin(tokenID, orgID string) error {
	res, err := DB.Exec("DELETE FROM user_access_tokens WHERE token_id = $1 AND org_id = $2", tokenID, orgID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("token not found in the organization")
	}
	return nil
}

// scanTokens는 sql.Rows를 AuthToken 슬라이스로 변환하는 헬퍼 함수입니다.
func scanTokens(rows *sql.Rows) ([]AuthToken, error) {
	var tokens []AuthToken
	for rows.Next() {
		var token AuthToken
		if err := rows.Scan(
			&token.TokenID,
			&token.UserID,
			&token.OrgID,
			&token.Description,
			&token.IsActive,
			&token.CreatedAt,
		); err != nil {
			log.Printf("Error scanning token row: %v\n", err)
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}
