package meta

import (
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

func ApplyMultipartPartSummary(upload *model.MultipartUpload, part model.MultipartPart, previous *model.MultipartPart, now time.Time) {
	if upload == nil {
		return
	}
	if previous == nil {
		upload.PartCount++
		upload.TotalPartSizeBytes += part.SizeBytes
	} else {
		upload.TotalPartSizeBytes += part.SizeBytes - previous.SizeBytes
	}
	if part.PartNumber > upload.MaxPartNumber {
		upload.MaxPartNumber = part.PartNumber
	}
	upload.PartsUpdatedAt = now
	upload.UpdatedAt = now
}
