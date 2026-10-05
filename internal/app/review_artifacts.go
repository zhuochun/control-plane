package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

type UploadReviewArtifact struct {
	RequestID string `json:"request_id,omitempty"`
	Data      string `json:"data_base64"`
}

type ReviewArtifact struct {
	ID        string `json:"id"`
	MediaType string `json:"media_type"`
	Size      int    `json:"size"`
}

func (a *App) UploadReviewArtifact(ctx context.Context, input UploadReviewArtifact) (json.RawMessage, error) {
	if len(input.Data) > 2800000 {
		return nil, Invalid("image exceeds 2 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil || len(data) > 2<<20 {
		return nil, Invalid("upload a base64 PNG or JPEG up to 2 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16000000 {
		return nil, Invalid("upload a valid PNG/JPEG up to 16 million pixels and 8192 pixels per dimension")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, Invalid("image data is incomplete or invalid")
	}
	sum := sha256.Sum256(data)
	artifact := ReviewArtifact{ID: hex.EncodeToString(sum[:]), MediaType: "image/" + format, Size: len(data)}
	return a.mutate(ctx, input.RequestID, "POST /review-artifacts", input, func(tx *sql.Tx) (any, error) {
		_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO review_artifacts(id,media_type,data) VALUES(?,?,?)", artifact.ID, artifact.MediaType, data)
		return artifact, err
	})
}

func (a *App) ReviewArtifact(ctx context.Context, id string) (string, []byte, error) {
	var media string
	var data []byte
	err := a.Store.DB.QueryRowContext(ctx, "SELECT media_type,data FROM review_artifacts WHERE id=?", id).Scan(&media, &data)
	return media, data, missing(err)
}
