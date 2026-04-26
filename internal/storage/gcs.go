package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// GCSBackend implements Backend using raw Google Cloud Storage JSON APIs
// via a 3-legged OAuth2 Client structure.
//
// Key advantage over GoogleBackend (Drive): GCS objects are addressed by name
// directly, so we don't need the fileIDs map. This eliminates a mutex, a memory
// leak vector, and simplifies Download/Delete to single-step operations.
type GCSBackend struct {
	httpClient *http.Client
	saPath     string // Path to client_secret_*.json (OAuth2 credentials)
	bucket     string // GCS bucket name

	clientID     string
	clientSecret string
	tokenURI     string
	redirectURI  string

	token        string
	refreshToken string
	tokenEx      time.Time
	mu           sync.Mutex
}

// NewGCSBackend creates a new GCSBackend.
func NewGCSBackend(client *http.Client, saPath, bucket string) *GCSBackend {
	return &GCSBackend{
		httpClient: client,
		saPath:     saPath,
		bucket:     bucket,
	}
}

func (b *GCSBackend) Login(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Parse Client Secret JSON
	data, err := os.ReadFile(b.saPath)
	if err != nil {
		return fmt.Errorf("failed to read Client Secret JSON %s: %w", b.saPath, err)
	}
	var oauthJSON oauthClientJSON
	if err := json.Unmarshal(data, &oauthJSON); err != nil {
		return fmt.Errorf("failed to unmarshal Client Secret JSON: %w", err)
	}

	b.clientID = oauthJSON.Installed.ClientID
	b.clientSecret = oauthJSON.Installed.ClientSecret
	// Hardcode TokenURI to www.googleapis.com to ensure domain fronting via Host header matches
	b.tokenURI = "https://www.googleapis.com/oauth2/v4/token"
	authURI := oauthJSON.Installed.AuthURI
	if len(oauthJSON.Installed.RedirectURIs) > 0 {
		b.redirectURI = oauthJSON.Installed.RedirectURIs[0]
	} else {
		b.redirectURI = "http://localhost"
	}

	tokenCachePath := b.saPath + ".token"

	// Check if we have a saved refresh token
	if cacheData, err := os.ReadFile(tokenCachePath); err == nil {
		var cache tokenCache
		if err := json.Unmarshal(cacheData, &cache); err == nil && cache.RefreshToken != "" {
			b.refreshToken = cache.RefreshToken
			return b.refreshAccessToken(ctx)
		}
	}

	// Interactive 3-Legged Flow — scope is devstorage.read_write for GCS
	link := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=https://www.googleapis.com/auth/devstorage.read_write&access_type=offline",
		authURI, url.QueryEscape(b.clientID), url.QueryEscape(b.redirectURI))

	fmt.Printf("\n==================== OAUTH AUTHENTICATION REQUIRED ====================\n")
	fmt.Printf("1. Please open this URL in your web browser:\n\n%s\n\n", link)
	fmt.Printf("2. Authenticate and accept the permissions.\n")
	fmt.Printf("3. The browser will redirect to something like %s/?code=4/1AX4X...\n", b.redirectURI)
	fmt.Printf("   (It's okay if the browser says 'Unable to connect' or 'Site can't be reached')\n")
	fmt.Printf("4. Please copy the FULL redirected URL from your browser's address bar and paste it below:\n")
	fmt.Printf("\nEnter URL or Code: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	// Extract code
	code := input
	if strings.HasPrefix(input, "http") {
		u, err := url.Parse(input)
		if err == nil {
			qCode := u.Query().Get("code")
			if qCode != "" {
				code = qCode
			}
		}
	}

	if code == "" {
		return fmt.Errorf("invalid authorization code")
	}

	fmt.Printf("Trading code for tokens via domain-fronted proxy mapping...\n")
	if err := b.exchangeCode(ctx, code); err != nil {
		return err
	}

	// Save the refresh token to sidestep interactive flow next boot
	cache := tokenCache{RefreshToken: b.refreshToken}
	cacheBytes, _ := json.MarshalIndent(cache, "", "  ")
	if err := os.WriteFile(tokenCachePath, cacheBytes, 0600); err != nil {
		fmt.Printf("WARNING: Failed to save refresh token to %s: %v\n", tokenCachePath, err)
	} else {
		fmt.Printf("Saved refresh token to %s. Future startups will be silent.\n", tokenCachePath)
	}

	fmt.Printf("OAuth Authentication Successful!\n=======================================================================\n\n")
	return nil
}

func (b *GCSBackend) exchangeCode(ctx context.Context, code string) error {
	v := url.Values{}
	v.Set("grant_type", "authorization_code")
	v.Set("code", code)
	v.Set("client_id", b.clientID)
	v.Set("client_secret", b.clientSecret)
	v.Set("redirect_uri", b.redirectURI)

	return b.executeTokenRequest(ctx, v)
}

func (b *GCSBackend) refreshAccessToken(ctx context.Context) error {
	v := url.Values{}
	v.Set("grant_type", "refresh_token")
	v.Set("refresh_token", b.refreshToken)
	v.Set("client_id", b.clientID)
	v.Set("client_secret", b.clientSecret)

	return b.executeTokenRequest(ctx, v)
}

func (b *GCSBackend) executeTokenRequest(ctx context.Context, v url.Values) error {
	req, err := http.NewRequestWithContext(ctx, "POST", b.tokenURI, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var resData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	b.token = resData.AccessToken
	if resData.RefreshToken != "" {
		b.refreshToken = resData.RefreshToken
	}
	b.tokenEx = time.Now().Add(time.Duration(resData.ExpiresIn-60) * time.Second) // 60s buffer
	return nil
}

func (b *GCSBackend) getValidToken(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if time.Now().After(b.tokenEx) {
		if err := b.refreshAccessToken(ctx); err != nil {
			return "", err
		}
	}
	return b.token, nil
}

// Upload writes an object to the GCS bucket using multipart upload.
// GCS endpoint: POST https://www.googleapis.com/upload/storage/v1/b/BUCKET/o?uploadType=multipart
func (b *GCSBackend) Upload(ctx context.Context, filename string, data io.Reader) error {
	tok, err := b.getValidToken(ctx)
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	metaWriter := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer metaWriter.Close()

		// Part 1: Metadata
		h := make(textproto.MIMEHeader)
		h.Set("Content-Type", "application/json; charset=UTF-8")
		part1, _ := metaWriter.CreatePart(h)
		meta := map[string]interface{}{
			"name": filename,
		}
		json.NewEncoder(part1).Encode(meta)

		// Part 2: Content
		h = make(textproto.MIMEHeader)
		h.Set("Content-Type", "application/octet-stream")
		part2, _ := metaWriter.CreatePart(h)
		io.Copy(part2, data)
	}()

	uploadURL := fmt.Sprintf("https://www.googleapis.com/upload/storage/v1/b/%s/o?uploadType=multipart",
		url.PathEscape(b.bucket))

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", metaWriter.FormDataContentType())

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListQuery lists objects in the bucket matching the given prefix.
// GCS natively supports prefix filtering — no client-side filtering needed.
// GCS endpoint: GET https://www.googleapis.com/storage/v1/b/BUCKET/o?prefix=PREFIX
func (b *GCSBackend) ListQuery(ctx context.Context, prefix string) ([]string, error) {
	tok, err := b.getValidToken(ctx)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s/o",
		url.PathEscape(b.bucket)))
	v := u.Query()
	v.Set("prefix", prefix)
	v.Set("fields", "items(name)")
	u.RawQuery = v.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list returned %d: %s", resp.StatusCode, string(body))
	}

	var resData struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return nil, err
	}

	var names []string
	for _, item := range resData.Items {
		names = append(names, item.Name)
	}

	return names, nil
}

// Download returns the content of an object from GCS.
// No fileID lookup needed — GCS addresses objects by name directly.
// GCS endpoint: GET https://www.googleapis.com/storage/v1/b/BUCKET/o/OBJECT?alt=media
func (b *GCSBackend) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	tok, err := b.getValidToken(ctx)
	if err != nil {
		return nil, err
	}

	downloadURL := fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s/o/%s?alt=media",
		url.PathEscape(b.bucket), url.PathEscape(filename))

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("download returned %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// Delete removes an object from GCS.
// No fileID lookup needed — GCS addresses objects by name directly.
// GCS endpoint: DELETE https://www.googleapis.com/storage/v1/b/BUCKET/o/OBJECT
func (b *GCSBackend) Delete(ctx context.Context, filename string) error {
	tok, err := b.getValidToken(ctx)
	if err != nil {
		return err
	}

	deleteURL := fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s/o/%s",
		url.PathEscape(b.bucket), url.PathEscape(filename))

	req, err := http.NewRequestWithContext(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateFolder is a no-op for GCS — objects use prefix-based "folder" structure.
// Returns the bucket name as the container ID.
func (b *GCSBackend) CreateFolder(ctx context.Context, name string) (string, error) {
	// GCS doesn't have real folders — objects are flat with prefix-based hierarchy.
	// The bucket itself is the container. Just return the bucket name.
	return b.bucket, nil
}

// FindFolder checks if the configured bucket exists and is accessible.
// Returns the bucket name if found, empty string otherwise.
// GCS endpoint: GET https://www.googleapis.com/storage/v1/b/BUCKET
func (b *GCSBackend) FindFolder(ctx context.Context, name string) (string, error) {
	tok, err := b.getValidToken(ctx)
	if err != nil {
		return "", err
	}

	bucketURL := fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s",
		url.PathEscape(b.bucket))

	req, err := http.NewRequestWithContext(ctx, "GET", bucketURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return b.bucket, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", nil // Bucket not found
	}

	body, _ := io.ReadAll(resp.Body)
	return "", fmt.Errorf("bucket check returned %d: %s", resp.StatusCode, string(body))
}
