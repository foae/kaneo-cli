package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foae/kaneo-cli/internal/client"
)

// maxAvatarBytes matches the documented 512 KiB avatar limit.
const maxAvatarBytes = 512 << 10

// newUploadCommands returns the two operations whose requests are constructed
// from a local file rather than passed through as a JSON body.
func (a *app) newUploadCommands() []groupedCommand {
	return []groupedCommand{
		{group: "task", cmd: a.newTaskImageUploadCommand()},
		{group: "user", cmd: a.newUserUploadAvatarCommand()},
	}
}

func readBodyFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, &processError{err: err}
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, &processError{err: err}
	}
	if !info.Mode().IsRegular() {
		return nil, &usageError{err: fmt.Errorf("%q is not a regular file", path)}
	}
	if info.Size() > maxRequestBody {
		return nil, &usageError{err: fmt.Errorf("request body exceeds %d bytes", maxRequestBody)}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRequestBody+1))
	if err != nil {
		return nil, &processError{err: err}
	}
	if len(data) > maxRequestBody {
		return nil, &usageError{err: fmt.Errorf("request body exceeds %d bytes", maxRequestBody)}
	}
	return data, nil
}

func contentTypeByExtension(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

func (a *app) newTaskImageUploadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-image-upload",
		Short: "Upload a task image through a presigned URL",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runTaskImageUpload(cmd)
		},
	}
	cmd.Flags().String("id", "", "Task ID (required)")
	cmd.Flags().String("file", "", "Local image file to upload (required)")
	cmd.Flags().String("surface", "", "Where the image is used: description or comment (required)")
	cmd.Flags().String("filename", "", "Filename to record; defaults to the file's base name")
	cmd.Flags().String("content-type", "", "Image content type; defaults from the file extension")
	return cmd
}

func (a *app) runTaskImageUpload(cmd *cobra.Command) error {
	taskID, _ := cmd.Flags().GetString("id")
	filePath, _ := cmd.Flags().GetString("file")
	surface, _ := cmd.Flags().GetString("surface")
	filename, _ := cmd.Flags().GetString("filename")
	contentType, _ := cmd.Flags().GetString("content-type")

	if taskID == "" {
		return &usageError{err: errors.New("--id is required")}
	}
	if taskID == "." || taskID == ".." {
		return &usageError{err: errors.New("--id must not be '.' or '..'")}
	}
	if filePath == "" {
		return &usageError{err: errors.New("--file is required")}
	}
	if surface != "description" && surface != "comment" {
		return &usageError{err: errors.New("--surface must be description or comment")}
	}
	if filename == "" {
		filename = filepath.Base(filePath)
	}
	if contentType == "" {
		contentType = contentTypeByExtension(filePath)
	}
	if contentType == "" {
		return &usageError{err: errors.New("--content-type is required for an unknown file extension")}
	}

	apiPath, err := expandPath("/task/image-upload/{id}", map[string]string{"id": taskID})
	if err != nil {
		return &processError{err: err}
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return &processError{err: err}
	}
	if info.IsDir() {
		return &usageError{err: fmt.Errorf("%q is a directory", filePath)}
	}
	size := info.Size()

	body, err := json.Marshal(map[string]any{
		"filename":    filename,
		"contentType": contentType,
		"size":        size,
		"surface":     surface,
	})
	if err != nil {
		return &processError{err: err}
	}

	ctx := cmd.Context()
	sess, err := a.session(ctx)
	if err != nil {
		return err
	}
	apiClient, err := sess.newClient(ctx)
	if err != nil {
		return err
	}
	resp, err := apiClient.Do(ctx, client.Request{
		Method:      "PUT",
		Path:        apiPath,
		Body:        body,
		ContentType: "application/json",
		OperationID: "createTaskImageUpload",
	})
	if err != nil {
		return err
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxRequestBody+1))
	_ = resp.Body.Close()
	if err != nil {
		return &processError{err: err}
	}
	if len(payload) > maxRequestBody {
		return &processError{err: fmt.Errorf("presigned upload response exceeds %d bytes", maxRequestBody)}
	}

	var upload struct {
		Key       string            `json:"key"`
		UploadURL string            `json:"uploadUrl"`
		Headers   map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(payload, &upload); err != nil {
		return &processError{err: client.ErrInvalidJSONResponse}
	}
	if upload.Key == "" {
		return &processError{err: errors.New("server did not return an upload key")}
	}

	if upload.UploadURL == "" {
		return &processError{err: errors.New("server did not return an upload URL")}
	}
	if err := a.putToStorage(ctx, sess, upload.UploadURL, upload.Headers, filePath, size); err != nil {
		return err
	}
	// Transfer credentials are consumed internally, never emitted to stdout.
	result, err := json.Marshal(struct {
		Key string `json:"key"`
	}{Key: upload.Key})
	if err != nil {
		return &processError{err: err}
	}
	return writeJSONBytes(cmd.OutOrStdout(), result)
}

// putToStorage streams the file to the presigned URL with a client that never
// carries the API credential.
func (a *app) putToStorage(ctx context.Context, sess *session, rawURL string, headers map[string]string, filePath string, size int64) error {
	target, err := validateStorageURL(rawURL)
	if err != nil {
		return err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return &processError{err: err}
	}
	defer func() { _ = file.Close() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, file)
	if err != nil {
		return &processError{err: err}
	}
	req.ContentLength = size
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	storage := &http.Client{
		Timeout: sess.resolved.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := storage.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, context.DeadlineExceeded) || client.IsTimeout(err) {
			return &client.TimeoutError{Err: err}
		}
		return &client.TransportError{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &processError{err: fmt.Errorf("storage upload failed with HTTP status %d", resp.StatusCode)}
	}
	return nil
}

// validateStorageURL accepts only an absolute http or https URL without
// userinfo, so a hostile presigned URL cannot smuggle credentials in.
func validateStorageURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", &processError{err: errors.New("server returned a malformed upload URL")}
	}
	if parsed.User != nil {
		return "", &processError{err: errors.New("server returned an upload URL with user information")}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", &processError{err: fmt.Errorf("upload URL must use http or https, got %q", parsed.Scheme)}
	}
	if parsed.Host == "" {
		return "", &processError{err: errors.New("server returned an upload URL with no host")}
	}
	return parsed.String(), nil
}

func (a *app) newUserUploadAvatarCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload-avatar",
		Short: "Upload the current user's avatar",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runUserUploadAvatar(cmd)
		},
	}
	cmd.Flags().String("file", "", "Local image file, PNG/JPEG/WebP up to 512 KiB (required)")
	cmd.Flags().String("content-type", "", "Image content type; defaults from the file extension")
	return cmd
}

func (a *app) runUserUploadAvatar(cmd *cobra.Command) error {
	filePath, _ := cmd.Flags().GetString("file")
	contentType, _ := cmd.Flags().GetString("content-type")
	if filePath == "" {
		return &usageError{err: errors.New("--file is required")}
	}
	if contentType == "" {
		contentType = contentTypeByExtension(filePath)
	}
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return &usageError{err: errors.New("--content-type must be image/png, image/jpeg or image/webp")}
	}
	data, err := readBodyFile(filePath)
	if err != nil {
		return err
	}
	if len(data) > maxAvatarBytes {
		return &usageError{err: fmt.Errorf("avatar exceeds %d bytes", maxAvatarBytes)}
	}

	body, err := json.Marshal(map[string]string{
		"contentType": contentType,
		"data":        base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return &processError{err: err}
	}

	ctx := cmd.Context()
	sess, err := a.session(ctx)
	if err != nil {
		return err
	}
	apiClient, err := sess.newClient(ctx)
	if err != nil {
		return err
	}
	resp, err := apiClient.Do(ctx, client.Request{
		Method:      "PUT",
		Path:        "/user/avatar",
		Body:        body,
		ContentType: "application/json",
		OperationID: "uploadUserAvatar",
	})
	if err != nil {
		return err
	}
	return writeJSONStream(cmd.OutOrStdout(), resp)
}
