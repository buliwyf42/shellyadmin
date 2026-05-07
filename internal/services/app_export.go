package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"shellyadmin/internal/models"
)

type DeviceExport struct {
	Version      int                `json:"version"`
	ExportedAt   string             `json:"exported_at"`
	Device       models.Device      `json:"device"`
	RawConfig    map[string]any     `json:"raw_config"`
	RawStatus    map[string]any     `json:"raw_status"`
	Capabilities []DeviceCapability `json:"capabilities"`
}

func (s *AppService) ExportDevice(target string) (DeviceExport, error) {
	detail, err := s.GetDeviceDetail(target)
	if err != nil {
		return DeviceExport{}, err
	}
	s.Log("INFO", fmt.Sprintf("device export requested target=%s mac=%s", target, detail.Device.MAC))
	return DeviceExport{
		Version:      1,
		ExportedAt:   time.Now().UTC().Format(time.RFC3339),
		Device:       detail.Device,
		RawConfig:    detail.RawConfig,
		RawStatus:    detail.RawStatus,
		Capabilities: detail.Capabilities,
	}, nil
}

const maxLogsForExport = 100000

func (s *AppService) ExportLogs(level, search, format string) (body []byte, filename, contentType string, err error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "ndjson" {
		return nil, "", "", errors.New("format must be csv or ndjson")
	}
	entries, err := s.db.GetLogsForExport(level, search, maxLogsForExport)
	if err != nil {
		return nil, "", "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	switch format {
	case "csv":
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		if err := w.Write([]string{"id", "ts", "level", "risk_level", "request_id", "message"}); err != nil {
			return nil, "", "", err
		}
		for _, entry := range entries {
			if err := w.Write([]string{strconv.Itoa(entry.ID), entry.TS, entry.Level, entry.RiskLevel, entry.RequestID, entry.Message}); err != nil {
				return nil, "", "", err
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, "", "", err
		}
		s.Log("INFO", fmt.Sprintf("logs export requested format=csv rows=%d", len(entries)))
		return buf.Bytes(), fmt.Sprintf("shellyadmin-logs-%s.csv", stamp), "text/csv; charset=utf-8", nil
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		for _, entry := range entries {
			if err := enc.Encode(entry); err != nil {
				return nil, "", "", err
			}
		}
		s.Log("INFO", fmt.Sprintf("logs export requested format=ndjson rows=%d", len(entries)))
		return buf.Bytes(), fmt.Sprintf("shellyadmin-logs-%s.ndjson", stamp), "application/x-ndjson", nil
	}
}
