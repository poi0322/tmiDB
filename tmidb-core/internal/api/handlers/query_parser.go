package handlers

import (
	"net/url"
	"regexp"
	"strings"
)

// Filter는 쿼리 필터를 나타내는 구조체입니다.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// QueryParser는 클라이언트 쿼리를 파싱하여 데이터베이스 함수에 전달할 형태로 변환합니다.
type QueryParser struct{}

// ParseQueryParams는 URL 쿼리 파라미터를 파싱하여 필터 배열로 변환합니다.
func (qp *QueryParser) ParseQueryParams(queryParams url.Values) ([]Filter, error) {
	var filters []Filter

	for key, values := range queryParams {
		if len(values) == 0 {
			continue
		}

		value := values[0]

		// 다양한 연산자 패턴 매칭
		operators := []struct {
			pattern string
			op      string
		}{
			{`^(.+)>=(.+)$`, ">="},
			{`^(.+)<=(.+)$`, "<="},
			{`^(.+)>(.+)$`, ">"},
			{`^(.+)<(.+)$`, "<"},
			{`^(.+)!=(.+)$`, "!="},
			{`^(.+)~(.+)$`, "~"},     // LIKE 검색
			{`^(.+)!~(.+)$`, "!~"},   // NOT LIKE 검색
			{`^(.+)!in(.+)$`, "!in"}, // NOT IN 연산자
		}

		matched := false
		for _, op := range operators {
			re := regexp.MustCompile(op.pattern)
			if matches := re.FindStringSubmatch(key + value); len(matches) == 3 {
				filters = append(filters, Filter{
					Field: strings.TrimSpace(matches[1]),
					Op:    op.op,
					Value: strings.TrimSpace(matches[2]),
				})
				matched = true
				break
			}
		}

		// 연산자가 없으면 특수 케이스 및 기본 처리
		if !matched {
			// 배열 관련 연산자들
			if strings.HasSuffix(key, "[]contains") {
				fieldName := strings.TrimSuffix(key, "[]contains")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]!contains") {
				fieldName := strings.TrimSuffix(key, "[]!contains")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "!array_includes",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]includes_any") {
				fieldName := strings.TrimSuffix(key, "[]includes_any")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes_any",
					Value: value,
				})
			} else if strings.HasSuffix(key, "[]includes_all") {
				fieldName := strings.TrimSuffix(key, "[]includes_all")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "array_includes_all",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size") {
				fieldName := strings.TrimSuffix(key, ".size")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size>") {
				fieldName := strings.TrimSuffix(key, ".size>")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size>",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size>=") {
				fieldName := strings.TrimSuffix(key, ".size>=")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size>=",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size<") {
				fieldName := strings.TrimSuffix(key, ".size<")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size<",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".size<=") {
				fieldName := strings.TrimSuffix(key, ".size<=")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "size<=",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".exists") {
				fieldName := strings.TrimSuffix(key, ".exists")
				if value == "true" || value == "1" {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "exists",
						Value: "true",
					})
				} else {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "!exists",
						Value: "true",
					})
				}
			} else if strings.HasSuffix(key, ".empty") {
				fieldName := strings.TrimSuffix(key, ".empty")
				if value == "true" || value == "1" {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "empty",
						Value: "true",
					})
				} else {
					filters = append(filters, Filter{
						Field: fieldName,
						Op:    "!empty",
						Value: "true",
					})
				}
			} else if strings.HasSuffix(key, ".like") {
				fieldName := strings.TrimSuffix(key, ".like")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "like",
					Value: value,
				})
			} else if strings.HasSuffix(key, ".regex") {
				fieldName := strings.TrimSuffix(key, ".regex")
				filters = append(filters, Filter{
					Field: fieldName,
					Op:    "regex",
					Value: value,
				})
			} else if strings.Contains(value, ",") {
				// 쉼표가 포함된 값은 IN 연산자로 처리
				filters = append(filters, Filter{
					Field: key,
					Op:    "in",
					Value: value,
				})
			} else {
				// 기본 등호 연산자
				filters = append(filters, Filter{
					Field: key,
					Op:    "=",
					Value: value,
				})
			}
		}
	}

	return filters, nil
}

// ParseMultiListenerPath는 다중 리스너 API 경로를 파싱합니다.
func ParseMultiListenerPath(path string) []string {
	// "/api/v1/listener/server_monitor/sensor_broken/air_sensor" -> ["server_monitor", "sensor_broken", "air_sensor"]
	parts := strings.Split(path, "/")
	if len(parts) < 5 || parts[1] != "api" || parts[3] != "listener" {
		return nil
	}
	return parts[4:] // listener 이후의 모든 부분
}
