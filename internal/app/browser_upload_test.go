package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
	"github.com/spf13/cobra"
)

type browserUploadFixture struct {
	key, contentType string
	size             int64
	private          bool
	providerError    error
}

func (f *browserUploadFixture) SignBrowserUpload(_ context.Context, key, contentType string, size int64, private bool) (oss.UploadForm, error) {
	f.key, f.contentType, f.size, f.private = key, contentType, size, private
	return oss.UploadForm{
		URL:       "https://oss.example.test",
		Fields:    map[string]string{"key": key, "x-oss-content-type": contentType},
		ExpiresAt: time.Date(2026, 11, 23, 12, 10, 0, 0, time.UTC),
	}, f.providerError
}

func (f *browserUploadFixture) InspectBrowserUpload(_ context.Context, key string) (oss.UploadedHeader, error) {
	f.key = key
	return oss.UploadedHeader{Exists: true, Size: 3, ContentType: "image/jpeg", Header: []byte{0xff, 0xd8, 0xff}}, f.providerError
}

func TestBrowserUploadCommandsAcceptFlagsAndReturnJSON(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	fixture := &browserUploadFixture{}
	factory := func(p profile.Profile) (browserUploadClient, error) {
		if p.Name != profileName {
			t.Fatalf("wrong profile %q", p.Name)
		}
		return fixture, nil
	}

	var stdout bytes.Buffer
	sign := newSignUploadCmdWithFactory(factory)
	sign.SetOut(&stdout)
	sign.SetArgs([]string{"--profile", profileName, "--key", "media/a.jpg", "--content-type", "image/jpeg", "--size", "3", "--private"})
	if err := sign.Execute(); err != nil {
		t.Fatal(err)
	}
	var signed oss.UploadForm
	if err := json.Unmarshal(stdout.Bytes(), &signed); err != nil {
		t.Fatal(err)
	}
	if fixture.key != "media/a.jpg" || fixture.contentType != "image/jpeg" || fixture.size != 3 || !fixture.private ||
		signed.URL != "https://oss.example.test" || signed.Fields["x-oss-content-type"] != "image/jpeg" || signed.ExpiresAt.IsZero() {
		t.Fatalf("unexpected sign I/O: %+v / %+v", fixture, signed)
	}

	stdout.Reset()
	inspect := newInspectUploadCmdWithFactory(factory)
	inspect.SetOut(&stdout)
	inspect.SetArgs([]string{"--profile", profileName, "--key", "media/b.jpg"})
	if err := inspect.Execute(); err != nil {
		t.Fatal(err)
	}
	var checked oss.UploadedHeader
	if err := json.Unmarshal(stdout.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if fixture.key != "media/b.jpg" || !checked.Exists || checked.Size != 3 ||
		checked.ContentType != "image/jpeg" || !bytes.Equal(checked.Header, []byte{0xff, 0xd8, 0xff}) {
		t.Fatalf("unexpected inspect I/O: %+v / %+v", fixture, checked)
	}
}

func TestBrowserUploadCommandsValidateInputAndSanitizeErrors(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"sign missing size", []string{"sign-upload", "--profile", profileName, "--key", "media/a.jpg", "--content-type", "image/jpeg"}, "size"},
		{"sign bad size flag", []string{"sign-upload", "--profile", profileName, "--key", "media/a.jpg", "--content-type", "image/jpeg", "--size", "bad"}, "INVALID_ARGUMENT"},
		{"sign oversized", []string{"sign-upload", "--profile", profileName, "--key", "media/a.jpg", "--content-type", "image/jpeg", "--size", "20971521"}, "OSS_SIGN_UPLOAD_ERROR"},
		{"inspect unsafe key", []string{"inspect-upload", "--profile", profileName, "--key", "../a.jpg"}, "INVALID_ARGUMENT"},
		{"inspect unsafe profile", []string{"inspect-upload", "--profile", "../private", "--key", "media/a.jpg"}, "INVALID_ARGUMENT"},
		{"inspect missing profile", []string{"inspect-upload", "--profile", "missing", "--key", "media/a.jpg"}, "PROFILE_ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}

	factory := func(profile.Profile) (browserUploadClient, error) {
		return &browserUploadFixture{providerError: errors.New("private-provider-error-canary")}, nil
	}
	for _, tt := range []struct {
		newCommand func(browserUploadClientFactory) *cobra.Command
		extraArgs  []string
		want       string
	}{
		{newSignUploadCmdWithFactory, []string{"--content-type", "image/jpeg", "--size", "3"}, "OSS_SIGN_UPLOAD_ERROR"},
		{newInspectUploadCmdWithFactory, nil, "OSS_INSPECT_UPLOAD_ERROR"},
	} {
		cmd := tt.newCommand(factory)
		cmd.SetArgs(append([]string{"--profile", profileName, "--key", "media/a.jpg"}, tt.extraArgs...))
		if err := cmd.Execute(); err == nil || err.Error() != tt.want {
			t.Fatalf("expected %q, got %v", tt.want, err)
		}
	}
}
