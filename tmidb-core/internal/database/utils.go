package database

import "os"

// getEnvOrDefault는 환경변수 값을 가져오거나 기본값을 반환합니다.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
