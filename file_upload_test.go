package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestSpoolAndHashUpload(t *testing.T) {
	t.Parallel()

	p := &Plugin{}
	spooledFile, fileHash, err := p.spoolAndHashUpload(strings.NewReader("policy file"), 1024)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer spooledFile.Close()

	if _, err := spooledFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("failed to rewind spooled file: %v", err)
	}
	content, err := io.ReadAll(spooledFile)
	if err != nil {
		t.Fatalf("failed to read spooled file: %v", err)
	}
	if string(content) != "policy file" {
		t.Fatalf("unexpected spooled content %q", string(content))
	}

	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte("policy file")))
	if fileHash != wantHash {
		t.Fatalf("expected hash %q, got %q", wantHash, fileHash)
	}
}

func TestSpoolAndHashUploadRejectsOversizedFile(t *testing.T) {
	t.Parallel()

	p := &Plugin{}
	spooledFile, fileHash, err := p.spoolAndHashUpload(strings.NewReader("123456"), 5)
	if err == nil {
		if spooledFile != nil {
			_ = spooledFile.Close()
		}
		t.Fatal("expected oversized file error")
	}
	if spooledFile != nil {
		t.Fatal("expected no spooled file for oversized upload")
	}
	if fileHash != "" {
		t.Fatalf("expected empty hash, got %q", fileHash)
	}
	if !strings.Contains(err.Error(), "maximum inspection size") {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestFileWillBeUploadedAllowsAndCopiesOutput(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)
	api.On("GetUser", "user-id").Return(&model.User{Id: "user-id", Username: "alice", Email: "alice@example.com"}, nil).Once()
	api.On("GetChannel", "channel-id").Return(&model.Channel{Id: "channel-id", Name: "general", Header: "header"}, nil).Once()
	api.On("GetPropertyGroup", model.CustomProfileAttributesPropertyGroupName).Return((*model.PropertyGroup)(nil), nil).Once()
	api.On("LogInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

	var policyRequest map[string]interface{}
	p := newPolicyTestPlugin("https://tdi.example.com")
	p.API = api
	p.setConfiguration(&configuration{
		TDIURL:                 "https://tdi.example.com",
		TDINamespace:           "mattermost-policies",
		PolicyTimeout:          5,
		EnableFileUploadPolicy: true,
		MaxFileInspectionBytes: 1024,
	})
	p.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ns/mattermost-policies/policy/v1/file/upload" {
			t.Fatalf("unexpected policy path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&policyRequest); err != nil {
			t.Fatalf("failed to decode policy request: %v", err)
		}
		return jsonResponse(http.StatusOK, `{"status":"success","action":"continue","result":{}}`), nil
	})}

	var output bytes.Buffer
	info := &model.FileInfo{
		CreatorId: "user-id",
		ChannelId: "channel-id",
		Name:      "report.txt",
		Size:      int64(len("file-body")),
		MimeType:  "text/plain",
	}
	gotInfo, reason := p.FileWillBeUploaded(nil, info, strings.NewReader("file-body"), &output)
	if reason != "" {
		t.Fatalf("expected allow, got reason %q", reason)
	}
	if gotInfo != info {
		t.Fatal("expected original file info")
	}
	if output.String() != "file-body" {
		t.Fatalf("unexpected output %q", output.String())
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte("file-body")))
	if policyRequest["file_hash"] != wantHash {
		t.Fatalf("expected policy hash %q, got %v", wantHash, policyRequest["file_hash"])
	}
}

func TestFileWillBeUploadedRejectsOversizedBeforePolicyCall(t *testing.T) {
	t.Parallel()

	api := &plugintest.API{}
	defer api.AssertExpectations(t)
	api.On("GetUser", "user-id").Return(&model.User{Id: "user-id", Username: "alice"}, nil).Once()
	api.On("GetChannel", "channel-id").Return(&model.Channel{Id: "channel-id", Name: "general"}, nil).Once()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything).Return().Once()

	p := newPolicyTestPlugin("https://tdi.example.com")
	p.API = api
	p.setConfiguration(&configuration{
		TDIURL:                 "https://tdi.example.com",
		TDINamespace:           "mattermost-policies",
		PolicyTimeout:          5,
		EnableFileUploadPolicy: true,
		MaxFileInspectionBytes: 5,
	})
	p.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("policy service should not be called for oversized uploads")
		return nil, nil
	})}

	var output bytes.Buffer
	info := &model.FileInfo{CreatorId: "user-id", ChannelId: "channel-id", Name: "large.bin", Size: 6}
	gotInfo, reason := p.FileWillBeUploaded(nil, info, strings.NewReader("123456"), &output)
	if gotInfo != nil {
		t.Fatal("expected upload to be rejected")
	}
	if !strings.Contains(reason, "maximum inspection size") {
		t.Fatalf("unexpected reason %q", reason)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no output, got %q", output.String())
	}
}
