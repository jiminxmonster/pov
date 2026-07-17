package app

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookieName = "pov_admin"

type Config struct {
	Port          string
	DatabaseURL   string
	AdminUsername string
	AdminPassword string
	SessionSecret string
	PublicOrigin  string
	BasePath      string
	UploadDir     string
}

type Server struct {
	config Config
	db     *pgxpool.Pool
	router http.Handler
}

type Post struct {
	ID           string            `json:"id"`
	Slug         string            `json:"slug"`
	Title        string            `json:"title"`
	BodyMarkdown string            `json:"body_markdown"`
	Metadata     map[string]string `json:"metadata"`
	Address      string            `json:"address"`
	Latitude     float64           `json:"latitude"`
	Longitude    float64           `json:"longitude"`
	ImageURL     string            `json:"image_url"`
	Status       string            `json:"status"`
	SourceType   string            `json:"source_type"`
	PublishedAt  *time.Time        `json:"published_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type searchResponse struct {
	Items          []Post `json:"items"`
	Interpretation string `json:"interpretation,omitempty"`
	Total          int    `json:"total"`
}

func ConfigFromEnv() Config {
	return Config{
		Port:          envOr("PORT", "8080"),
		DatabaseURL:   envOr("DATABASE_URL", "postgres://pov:pov-local-password@localhost:5432/pov?sslmode=disable"),
		AdminUsername: envOr("ADMIN_USERNAME", "admin"),
		AdminPassword: envOr("ADMIN_PASSWORD", "admin"),
		SessionSecret: envOr("SESSION_SECRET", "local-development-secret-change-before-deploy"),
		PublicOrigin:  envOr("PUBLIC_ORIGIN", "http://localhost:3000"),
		BasePath:      normalizeBasePath(envOr("APP_BASE_PATH", "/")),
		UploadDir:     envOr("UPLOAD_DIR", "./uploads"),
	}
}

func New(ctx context.Context, config Config) (*Server, error) {
	config.BasePath = normalizeBasePath(config.BasePath)
	if err := os.MkdirAll(config.UploadDir, 0o750); err != nil {
		return nil, fmt.Errorf("create upload directory: %w", err)
	}

	var pool *pgxpool.Pool
	var err error
	for attempt := 1; attempt <= 20; attempt++ {
		pool, err = pgxpool.New(ctx, config.DatabaseURL)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	server := &Server{config: config, db: pool}
	if err := server.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	server.router = server.routes()
	return server, nil
}

func (s *Server) Close() {
	s.db.Close()
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Use(s.cors)

	router.Get("/health", s.health)
	router.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.config.UploadDir))))

	router.Route("/api/v1", func(api chi.Router) {
		api.Get("/posts", s.listPublishedPosts)
		api.Get("/posts/{slug}", s.getPublishedPost)
		api.Get("/map/posts", s.listPublishedPosts)
		api.Post("/search/ai", s.aiSearch)
		api.Post("/submissions", s.createSubmission)

		api.Route("/admin", func(admin chi.Router) {
			admin.Post("/auth/login", s.login)
			admin.Post("/auth/logout", s.logout)
			admin.Group(func(private chi.Router) {
				private.Use(s.requireAdmin)
				private.Get("/session", s.adminSession)
				private.Get("/posts", s.listAdminPosts)
				private.Post("/posts", s.createPost)
				private.Post("/posts/{id}/publish", s.publishPost)
				private.Post("/media", s.uploadMedia)
				private.Post("/uploads", s.importDocument)
			})
		})
	})
	return router
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listPublishedPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := s.queryPosts(r.Context(), r.URL.Query().Get("q"), "published", r.URL.Query().Get("bbox"), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "게시글을 불러오지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{Items: posts, Total: len(posts)})
}

func (s *Server) getPublishedPost(w http.ResponseWriter, r *http.Request) {
	post, err := s.getPost(r.Context(), "slug", chi.URLParam(r, "slug"), true)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "게시글을 찾을 수 없습니다")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "게시글을 불러오지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, post)
}

func (s *Server) aiSearch(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Query string `json:"query"`
		BBox  string `json:"bbox"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	posts, err := s.queryPosts(r.Context(), input.Query, "published", input.BBox, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "검색 중 문제가 발생했습니다")
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{
		Items:          posts,
		Total:          len(posts),
		Interpretation: interpretQuery(input.Query),
	})
}

func (s *Server) listAdminPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := s.queryPosts(r.Context(), r.URL.Query().Get("q"), "", "", 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "게시글을 불러오지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{Items: posts, Total: len(posts)})
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BodyMarkdown string `json:"body_markdown"`
		ImageURL     string `json:"image_url"`
		Publish      bool   `json:"publish"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.BodyMarkdown = strings.TrimSpace(input.BodyMarkdown)
	if len(input.BodyMarkdown) < 20 {
		writeError(w, http.StatusBadRequest, "게시글 내용을 입력해 주세요")
		return
	}

	metadata, title, address, latitude, longitude := parseTemplate(input.BodyMarkdown)
	metadataBytes, _ := json.Marshal(metadata)
	status := "review"
	var publishedAt any
	if input.Publish {
		status = "published"
		publishedAt = time.Now()
	}

	row := s.db.QueryRow(r.Context(), `
		INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'manual', $10)
		RETURNING id, slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at, created_at, updated_at
	`, newSlug(), title, input.BodyMarkdown, metadataBytes, address, latitude, longitude, input.ImageURL, status, publishedAt)

	post, err := scanPost(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "게시글을 저장하지 못했습니다")
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

func (s *Server) createSubmission(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "제보 내용을 읽지 못했습니다")
		return
	}
	if strings.TrimSpace(r.FormValue("website")) != "" {
		writeError(w, http.StatusBadRequest, "제보를 접수하지 못했습니다")
		return
	}

	body := strings.TrimSpace(r.FormValue("body_markdown"))
	if len([]rune(body)) < 20 {
		writeError(w, http.StatusBadRequest, "전시 정보를 조금 더 입력해 주세요")
		return
	}

	metadata, title, address, latitude, longitude := parseTemplate(body)
	if title == "제목 확인 필요" || address == "" {
		writeError(w, http.StatusBadRequest, "전시명과 장소를 입력해 주세요")
		return
	}
	metadataBytes, _ := json.Marshal(metadata)

	imageURL := ""
	file, header, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		name, saveErr := s.saveImageUpload(file, header)
		if saveErr != nil {
			writeError(w, http.StatusBadRequest, saveErr.Error())
			return
		}
		imageURL = prefixedPath(s.config.BasePath, "/uploads/"+name)
	} else if !errors.Is(err, http.ErrMissingFile) {
		writeError(w, http.StatusBadRequest, "대표 이미지를 읽지 못했습니다")
		return
	}

	row := s.db.QueryRow(r.Context(), `
		INSERT INTO posts (slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'review', 'community')
		RETURNING id, slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at, created_at, updated_at
	`, newSlug(), title, body, metadataBytes, address, latitude, longitude, imageURL)

	post, err := scanPost(row)
	if err != nil {
		if imageURL != "" {
			_ = os.Remove(filepath.Join(s.config.UploadDir, filepath.Base(imageURL)))
		}
		writeError(w, http.StatusInternalServerError, "제보를 저장하지 못했습니다")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      post.ID,
		"message": "전시 정보가 접수되었습니다. 운영자 확인 후 공개됩니다.",
	})
}

func (s *Server) publishPost(w http.ResponseWriter, r *http.Request) {
	row := s.db.QueryRow(r.Context(), `
		UPDATE posts SET status = 'published', published_at = NOW(), updated_at = NOW()
		WHERE id = $1
		RETURNING id, slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at, created_at, updated_at
	`, chi.URLParam(r, "id"))
	post, err := scanPost(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "게시글을 찾을 수 없습니다")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "게시하지 못했습니다")
		return
	}
	writeJSON(w, http.StatusOK, post)
}

func (s *Server) queryPosts(ctx context.Context, query, status, bbox string, limit int) ([]Post, error) {
	west, south, east, north, hasBBox := parseBBox(bbox)
	terms := searchTerms(query)
	rows, err := s.db.Query(ctx, `
		SELECT id, slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at, created_at, updated_at
		FROM posts
		WHERE ($1 = '' OR status = $1)
		  AND (cardinality($2::text[]) = 0 OR NOT EXISTS (
			SELECT 1 FROM unnest($2::text[]) AS term
			WHERE NOT (title ILIKE '%' || term || '%' OR body_markdown ILIKE '%' || term || '%' OR address ILIKE '%' || term || '%')
		  ))
		  AND (NOT $3 OR (longitude BETWEEN $4 AND $6 AND latitude BETWEEN $5 AND $7))
		ORDER BY COALESCE(published_at, created_at) DESC
		LIMIT $8
	`, status, terms, hasBBox, west, south, east, north, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPost(row rowScanner) (Post, error) {
	var post Post
	var metadataBytes []byte
	err := row.Scan(
		&post.ID, &post.Slug, &post.Title, &post.BodyMarkdown, &metadataBytes,
		&post.Address, &post.Latitude, &post.Longitude, &post.ImageURL, &post.Status,
		&post.SourceType, &post.PublishedAt, &post.CreatedAt, &post.UpdatedAt,
	)
	if err != nil {
		return Post{}, err
	}
	if err := json.Unmarshal(metadataBytes, &post.Metadata); err != nil {
		post.Metadata = map[string]string{}
	}
	return post, nil
}

func (s *Server) getPost(ctx context.Context, field, value string, publishedOnly bool) (Post, error) {
	if field != "slug" && field != "id" {
		return Post{}, errors.New("unsupported post lookup")
	}
	publishedClause := ""
	if publishedOnly {
		publishedClause = " AND status = 'published'"
	}
	query := fmt.Sprintf(`
		SELECT id, slug, title, body_markdown, metadata, address, latitude, longitude, image_url, status, source_type, published_at, created_at, updated_at
		FROM posts WHERE %s = $1%s
	`, field, publishedClause)
	return scanPost(s.db.QueryRow(ctx, query, value))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	usernameMatches := subtle.ConstantTimeCompare([]byte(input.Username), []byte(s.config.AdminUsername)) == 1
	passwordMatches := subtle.ConstantTimeCompare([]byte(input.Password), []byte(s.config.AdminPassword)) == 1
	if !usernameMatches || !passwordMatches {
		time.Sleep(350 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "로그인 정보가 올바르지 않습니다")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.createSession(input.Username, time.Now().Add(12*time.Hour)),
		Path:     s.config.BasePath,
		MaxAge:   int((12 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.config.PublicOrigin, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": input.Username})
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     s.config.BasePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.config.PublicOrigin, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) adminSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": s.config.AdminUsername})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || !s.validSession(cookie.Value) {
			writeError(w, http.StatusUnauthorized, "관리자 로그인이 필요합니다")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) createSession(username string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s|%d", username, expiresAt.Unix())
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	signed := payload + "|" + hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(signed))
}

func (s *Server) validSession(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || parts[0] != s.config.AdminUsername {
		return false
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expiresAt {
		return false
	}
	payload := parts[0] + "|" + parts[1]
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	expected, err := hex.DecodeString(parts[2])
	return err == nil && hmac.Equal(mac.Sum(nil), expected)
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "이미지 파일을 선택해 주세요")
		return
	}
	defer file.Close()

	name, err := s.saveImageUpload(file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": prefixedPath(s.config.BasePath, "/uploads/"+name)})
}

func normalizeBasePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return "/"
	}
	return "/" + strings.Trim(value, "/")
}

func prefixedPath(basePath, path string) string {
	basePath = normalizeBasePath(basePath)
	if basePath == "/" {
		return "/" + strings.TrimLeft(path, "/")
	}
	return basePath + "/" + strings.TrimLeft(path, "/")
}

func (s *Server) importDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "자료 파일을 선택해 주세요")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "파일을 읽지 못했습니다")
		return
	}
	extension := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".txt": true, ".md": true, ".csv": true, ".xlsx": true, ".docx": true, ".pdf": true}
	if !allowed[extension] {
		writeError(w, http.StatusBadRequest, "지원하지 않는 파일 형식입니다")
		return
	}
	if _, err := s.saveUploadBytes(data, extension); err != nil {
		writeError(w, http.StatusInternalServerError, "원본 파일을 보관하지 못했습니다")
		return
	}

	rawText, extractionNote := extractText(header.Filename, data)
	body := normalizeToTemplate(rawText, header.Filename)
	writeJSON(w, http.StatusOK, map[string]string{
		"body_markdown": body,
		"message":       extractionNote,
	})
}

func (s *Server) saveUpload(file multipart.File, extension string) (string, error) {
	name := uniqueName(extension)
	target, err := os.OpenFile(filepath.Join(s.config.UploadDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", err
	}
	defer target.Close()
	_, err = io.Copy(target, file)
	return name, err
}

func (s *Server) saveImageUpload(file multipart.File, header *multipart.FileHeader) (string, error) {
	extension := strings.ToLower(filepath.Ext(header.Filename))
	allowedExtensions := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	if !allowedExtensions[extension] {
		return "", errors.New("JPG, PNG, WebP 또는 GIF 이미지만 업로드할 수 있습니다")
	}

	signature := make([]byte, 512)
	n, err := file.Read(signature)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("이미지를 확인하지 못했습니다")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", errors.New("이미지를 확인하지 못했습니다")
	}
	contentType := http.DetectContentType(signature[:n])
	allowedContentTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
	if !allowedContentTypes[contentType] {
		return "", errors.New("올바른 이미지 파일을 선택해 주세요")
	}
	name, err := s.saveUpload(file, extension)
	if err != nil {
		return "", errors.New("이미지를 저장하지 못했습니다")
	}
	return name, nil
}

func (s *Server) saveUploadBytes(data []byte, extension string) (string, error) {
	name := uniqueName(extension)
	err := os.WriteFile(filepath.Join(s.config.UploadDir, name), data, 0o640)
	return name, err
}

func uniqueName(extension string) string {
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%d-%s%s", time.Now().Unix(), hex.EncodeToString(random), extension)
}

func extractText(filename string, data []byte) (string, string) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".md", ".csv":
		return string(data), "자료를 읽어 고정 양식에 배치했습니다. 내용을 검수해 주세요."
	case ".docx":
		return extractXMLFromZip(data, []string{"word/document.xml"}), "Word 문서에서 텍스트를 추출했습니다. 표와 이미지 위치를 검수해 주세요."
	case ".xlsx":
		return extractXMLFromZip(data, []string{"xl/sharedStrings.xml", "xl/worksheets/"}), "Excel 셀 내용을 추출했습니다. 게시물 구분과 날짜를 검수해 주세요."
	case ".pdf":
		return "", "PDF 원본을 보관했습니다. 이미지형 PDF는 OCR 연동 후 자동 변환되며, 지금은 본문을 확인해 주세요."
	default:
		return "", "원본을 보관했습니다."
	}
}

func extractXMLFromZip(data []byte, prefixes []string) string {
	reader, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return ""
	}
	var output strings.Builder
	for _, file := range reader.File {
		matched := false
		for _, prefix := range prefixes {
			if file.Name == prefix || strings.HasPrefix(file.Name, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			continue
		}
		decoder := xml.NewDecoder(stream)
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if chars, ok := token.(xml.CharData); ok {
				text := strings.TrimSpace(string(chars))
				if text != "" {
					output.WriteString(text)
					output.WriteString("\n")
				}
			}
		}
		_ = stream.Close()
	}
	return output.String()
}

func normalizeToTemplate(rawText, filename string) string {
	rawText = strings.TrimSpace(rawText)
	labels := templateLabels()
	if strings.HasPrefix(rawText, "전시명:") {
		body := rawText
		for _, label := range labels {
			if !strings.Contains(body, label+":") {
				body += "\n\n" + label + ":"
			}
		}
		return body + "\n"
	}

	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if rawText != "" {
		for _, line := range strings.Split(rawText, "\n") {
			if strings.TrimSpace(line) != "" {
				title = strings.TrimSpace(line)
				break
			}
		}
	}
	values := map[string]string{"전시명": title, "전시내용": rawText}
	var body strings.Builder
	for _, label := range labels {
		body.WriteString(label)
		body.WriteString(":\n")
		body.WriteString(values[label])
		body.WriteString("\n\n")
	}
	return strings.TrimSpace(body.String()) + "\n"
}

func parseTemplate(body string) (map[string]string, string, string, float64, float64) {
	labels := templateLabels()
	metadata := make(map[string]string, len(labels)+1)
	current := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		matched := false
		for _, label := range labels {
			prefix := label + ":"
			if strings.HasPrefix(trimmed, prefix) {
				current = label
				metadata[current] = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				matched = true
				break
			}
		}
		if matched || trimmed == "" || current == "" {
			continue
		}
		if metadata[current] != "" {
			metadata[current] += "\n"
		}
		metadata[current] += trimmed
	}

	title := strings.TrimSpace(metadata["전시명"])
	if title == "" {
		title = "제목 확인 필요"
	}
	address := strings.TrimSpace(metadata["장소"])
	latitude, longitude, locationStatus := approximateLocation(address)
	metadata["위치상태"] = locationStatus
	return metadata, title, address, latitude, longitude
}

func templateLabels() []string {
	return []string{
		"전시명", "작가(작가소개)", "관람료", "전시기간", "장소", "도슨트(전시장 가이드) 유무",
		"찾아가는 방법", "주차정보", "전시내용", "굿즈샵정보", "주변에 함께 볼 만한 전시",
		"주변에 볼거리", "맛집", "감상평", "페르소나 정보입력",
	}
}

func approximateLocation(address string) (float64, float64, string) {
	locations := []struct {
		keyword   string
		latitude  float64
		longitude float64
	}{
		{"성수", 37.5445, 127.0560},
		{"강남", 37.4979, 127.0276},
		{"삼청", 37.5824, 126.9810},
		{"홍대", 37.5572, 126.9254},
		{"한남", 37.5345, 127.0006},
		{"을지로", 37.5660, 126.9910},
	}
	for _, location := range locations {
		if strings.Contains(address, location.keyword) {
			return location.latitude, location.longitude, "지역 중심 좌표 — 검수 필요"
		}
	}
	return 37.5665, 126.9780, "서울 중심 좌표 — 주소 검수 필요"
}

func interpretQuery(query string) string {
	if strings.TrimSpace(query) == "" {
		return "현재 공개 중인 장면을 모았습니다."
	}
	conditions := make([]string, 0, 4)
	for _, condition := range []string{"무료", "주차", "도슨트", "이번 주말", "아이", "사진", "성수", "강남"} {
		if strings.Contains(query, condition) {
			conditions = append(conditions, condition)
		}
	}
	if len(conditions) == 0 {
		return fmt.Sprintf("‘%s’와 가까운 기록을 찾았습니다.", query)
	}
	return strings.Join(conditions, " · ") + " 조건을 중심으로 찾았습니다."
}

func searchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return []string{}
	}

	known := []string{"무료", "주차", "도슨트", "아이", "사진", "성수", "강남", "삼청", "홍대", "한남", "을지로"}
	terms := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, term := range known {
		if strings.Contains(query, term) {
			terms = append(terms, term)
			seen[term] = true
		}
	}
	if len(terms) > 0 {
		return terms
	}

	stopwords := map[string]bool{
		"이번": true, "주말": true, "주말에": true, "전시": true, "공연": true,
		"보고": true, "싶은": true, "있는": true, "함께": true, "관람": true,
	}
	for _, term := range strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '?' || r == '!' || r == '/' || r == '\n' || r == '\t'
	}) {
		term = strings.TrimSpace(term)
		for _, suffix := range []string{"에서", "으로", "에는", "에게", "부터", "까지"} {
			term = strings.TrimSuffix(term, suffix)
		}
		if len([]rune(term)) < 2 || stopwords[term] || seen[term] {
			continue
		}
		terms = append(terms, term)
		seen[term] = true
		if len(terms) == 5 {
			break
		}
	}
	return terms
}

func parseBBox(value string) (west, south, east, north float64, ok bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}
	values := make([]float64, 4)
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		values[index] = parsed
	}
	return values[0], values[1], values[2], values[3], true
}

func newSlug() string {
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return fmt.Sprintf("pov-%s-%s", time.Now().Format("20060102-150405"), hex.EncodeToString(random))
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (origin == s.config.PublicOrigin || origin == "http://localhost:3000") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "요청 형식을 확인해 주세요")
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
