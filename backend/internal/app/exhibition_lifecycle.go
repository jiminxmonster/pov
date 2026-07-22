package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const publicExpiredExhibitionMonths = 1

var exhibitionDatePattern = regexp.MustCompile(`\d{4}\s*[./-]\s*\d{1,2}\s*[./-]\s*\d{1,2}`)

func exhibitionEndDate(post Post) (time.Time, bool) {
	raw := strings.TrimSpace(post.Metadata["전시종료일"])
	if raw == "" {
		matches := exhibitionDatePattern.FindAllString(post.Metadata["전시기간"], -1)
		if len(matches) > 0 {
			raw = matches[len(matches)-1]
		}
	}
	return parseExhibitionDate(raw)
}

func parseExhibitionDate(value string) (time.Time, bool) {
	value = strings.ReplaceAll(strings.NewReplacer(".", "-", "/", "-").Replace(strings.TrimSpace(value)), " ", "")
	var year, month, day int
	if count, err := fmt.Sscanf(value, "%d-%d-%d", &year, &month, &day); err != nil || count != 3 {
		return time.Time{}, false
	}
	location := time.FixedZone("KST", 9*60*60)
	parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, location)
	if parsed.Year() != year || int(parsed.Month()) != month || parsed.Day() != day {
		return time.Time{}, false
	}
	return parsed, true
}

func exhibitionDayStart(value time.Time) time.Time {
	location := time.FixedZone("KST", 9*60*60)
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

func isExhibitionExpiredAt(post Post, now time.Time) bool {
	endDate, ok := exhibitionEndDate(post)
	return ok && endDate.Before(exhibitionDayStart(now))
}

func isPublicIndexExhibitionAt(post Post, now time.Time) bool {
	endDate, ok := exhibitionEndDate(post)
	if !ok {
		return true
	}
	cutoff := exhibitionDayStart(now).AddDate(0, -publicExpiredExhibitionMonths, 0)
	return !endDate.Before(cutoff)
}

func filterExhibitions(posts []Post, limit int, keep func(Post) bool) []Post {
	filtered := make([]Post, 0, min(len(posts), limit))
	for _, post := range posts {
		if !keep(post) {
			continue
		}
		filtered = append(filtered, post)
		if len(filtered) == limit {
			break
		}
	}
	return filtered
}

func publicIndexExhibitions(posts []Post, now time.Time, limit int) []Post {
	current := make([]Post, 0, min(len(posts), limit))
	ended := make([]Post, 0, min(len(posts), limit))
	undated := make([]Post, 0, min(len(posts), limit))
	for _, post := range posts {
		if !isPublicIndexExhibitionAt(post, now) {
			continue
		}
		if _, ok := exhibitionEndDate(post); !ok {
			undated = append(undated, post)
		} else if isExhibitionExpiredAt(post, now) {
			ended = append(ended, post)
		} else {
			current = append(current, post)
		}
	}
	current = append(current, ended...)
	current = append(current, undated...)
	if len(current) > limit {
		current = current[:limit]
	}
	return current
}

func currentExhibitions(posts []Post, now time.Time, limit int) []Post {
	current := make([]Post, 0, min(len(posts), limit))
	undated := make([]Post, 0, min(len(posts), limit))
	for _, post := range posts {
		if _, ok := exhibitionEndDate(post); !ok {
			undated = append(undated, post)
		} else if !isExhibitionExpiredAt(post, now) {
			current = append(current, post)
		}
	}
	current = append(current, undated...)
	if len(current) > limit {
		current = current[:limit]
	}
	return current
}

func currentMapExhibitions(posts []Post, now time.Time, limit int) []Post {
	return filterExhibitions(currentExhibitions(posts, now, len(posts)), limit, func(post Post) bool {
		return hasMappableLocation(post)
	})
}

func hasMappableLocation(post Post) bool {
	if strings.EqualFold(strings.TrimSpace(post.Metadata["지도표시"]), "아니오") {
		return false
	}
	return post.Latitude >= 33 && post.Latitude <= 39 && post.Longitude >= 124 && post.Longitude <= 132
}

func historicalKnowledgeExhibitions(posts []Post, now time.Time, limit int) []Post {
	past := make([]Post, 0, min(len(posts), limit))
	current := make([]Post, 0, min(len(posts), limit))
	for _, post := range posts {
		if isExhibitionExpiredAt(post, now) {
			past = append(past, post)
		} else {
			current = append(current, post)
		}
	}
	past = append(past, current...)
	if len(past) > limit {
		past = past[:limit]
	}
	return past
}
