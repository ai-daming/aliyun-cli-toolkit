package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"

	mediaoss "github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

type commandFactory func() *cobra.Command

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type recordingMediaClient struct {
	listPrefix string
	listCursor string
	listLimit  int
	deleteKey  string
	listResult mediaoss.ListResult
	listErr    error
	deleteErr  error
}

func (c *recordingMediaClient) List(_ context.Context, prefix, cursor string, limit int) (mediaoss.ListResult, error) {
	c.listPrefix, c.listCursor, c.listLimit = prefix, cursor, limit
	return c.listResult, c.listErr
}

func (c *recordingMediaClient) Delete(_ context.Context, key string) error {
	c.deleteKey = key
	return c.deleteErr
}

func saveCommandTestProfile(t *testing.T) string {
	t.Helper()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	p := profile.Profile{
		Name:            "component-test",
		Bucket:          "bucket",
		Region:          "cn-test",
		RoleArn:         "acs:ram::123:role/test",
		AccessKeyID:     "TEST_ACCESS_KEY",
		AccessKeySecret: "TEST_ACCESS_SECRET",
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	return p.Name
}

func TestListCommandPrintsStableJSON(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	next := "cursor-2"
	client := &recordingMediaClient{listResult: mediaoss.ListResult{
		Objects: []mediaoss.ObjectSummary{{
			Key:          "媒体/example.jpg",
			LastModified: "2026-08-18T10:00:00Z",
			Size:         123,
			ETag:         "etag",
		}},
		NextCursor: &next,
	}}
	cmd := newListCmdWithFactory(func(profile.Profile) (mediaClient, error) { return client, nil })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"--profile", profileName,
		"--prefix", "媒体/",
		"--limit", "200",
		"--cursor", "cursor-1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "{\"objects\":[{\"key\":\"媒体/example.jpg\",\"lastModified\":\"2026-08-18T10:00:00Z\",\"size\":123,\"etag\":\"etag\"}],\"nextCursor\":\"cursor-2\"}\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if client.listPrefix != "媒体/" || client.listCursor != "cursor-1" || client.listLimit != 200 {
		t.Fatalf("list args = %q, %q, %d", client.listPrefix, client.listCursor, client.listLimit)
	}
}

func TestListCommandPrintsNullCursorAtEnd(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	client := &recordingMediaClient{listResult: mediaoss.ListResult{Objects: []mediaoss.ObjectSummary{}}}
	cmd := newListCmdWithFactory(func(profile.Profile) (mediaClient, error) { return client, nil })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--profile", profileName, "--prefix", "media", "--limit", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stdout.String() != "{\"objects\":[],\"nextCursor\":null}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDeleteCommandReturnsIdempotentPostcondition(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	client := &recordingMediaClient{}
	cmd := newDeleteCmdWithFactory(func(profile.Profile) (mediaClient, error) { return client, nil })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--profile", profileName, "--key", "media/example.jpg"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stdout.String() != "{\"key\":\"media/example.jpg\",\"deleted\":true}\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if client.deleteKey != "media/example.jpg" {
		t.Fatalf("delete key = %q", client.deleteKey)
	}
}

func TestListDeleteCommandsReturnSafeErrors(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	secretCanary := "PROVIDER_SECRET_CANARY"
	client := &recordingMediaClient{
		listErr:   errors.New(secretCanary),
		deleteErr: errors.New(secretCanary),
	}
	factory := func(profile.Profile) (mediaClient, error) { return client, nil }

	listCmd := newListCmdWithFactory(factory)
	listCmd.SetArgs([]string{"--profile", profileName, "--prefix", "media/", "--limit", "1"})
	if err := listCmd.Execute(); err == nil || err.Error() != "OSS_LIST_ERROR" {
		t.Fatalf("list error = %v", err)
	}

	deleteCmd := newDeleteCmdWithFactory(factory)
	deleteCmd.SetArgs([]string{"--profile", profileName, "--key", "media/example.jpg"})
	if err := deleteCmd.Execute(); err == nil || err.Error() != "OSS_DELETE_ERROR" {
		t.Fatalf("delete error = %v", err)
	}
}

func TestListDeleteCommandsRejectArgumentsBeforeLoadingProfile(t *testing.T) {
	tests := []struct {
		name string
		cmd  commandFactory
		args []string
	}{
		{name: "list missing profile", cmd: newListCmd, args: []string{"--prefix", "media/", "--limit", "1"}},
		{name: "list unsafe prefix", cmd: newListCmd, args: []string{"--profile", "missing", "--prefix", "../", "--limit", "1"}},
		{name: "list bad limit", cmd: newListCmd, args: []string{"--profile", "missing", "--prefix", "media/", "--limit", "abc"}},
		{name: "list empty cursor", cmd: newListCmd, args: []string{"--profile", "missing", "--prefix", "media/", "--limit", "1", "--cursor", ""}},
		{name: "list unknown flag", cmd: newListCmd, args: []string{"--unknown"}},
		{name: "list positional argument", cmd: newListCmd, args: []string{"unexpected"}},
		{name: "delete missing key", cmd: newDeleteCmd, args: []string{"--profile", "missing"}},
		{name: "delete unsafe key", cmd: newDeleteCmd, args: []string{"--profile", "missing", "--key", "media/../secret"}},
		{name: "delete unknown flag", cmd: newDeleteCmd, args: []string{"--unknown"}},
		{name: "delete positional argument", cmd: newDeleteCmd, args: []string{"unexpected"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil || err.Error() != "INVALID_ARGUMENT" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestListCommandMapsProfileAndFactoryFailures(t *testing.T) {
	cmd := newListCmd()
	cmd.SetArgs([]string{"--profile", "missing", "--prefix", "media/", "--limit", "1"})
	if err := cmd.Execute(); err == nil || err.Error() != "PROFILE_ERROR" {
		t.Fatalf("missing profile error = %v", err)
	}

	profileName := saveCommandTestProfile(t)
	factoryCanary := errors.New("FACTORY_SECRET_CANARY")
	cmd = newListCmdWithFactory(func(profile.Profile) (mediaClient, error) { return nil, factoryCanary })
	cmd.SetArgs([]string{"--profile", profileName, "--prefix", "media/", "--limit", "1"})
	if err := cmd.Execute(); err == nil || err.Error() != "PROFILE_ERROR" {
		t.Fatalf("factory error = %v", err)
	}
}

func TestDeleteCommandMapsProfileAndFactoryFailures(t *testing.T) {
	cmd := newDeleteCmd()
	cmd.SetArgs([]string{"--profile", "missing", "--key", "media/file"})
	if err := cmd.Execute(); err == nil || err.Error() != "PROFILE_ERROR" {
		t.Fatalf("missing profile error = %v", err)
	}

	profileName := saveCommandTestProfile(t)
	cmd = newDeleteCmdWithFactory(func(profile.Profile) (mediaClient, error) {
		return nil, errors.New("FACTORY_SECRET_CANARY")
	})
	cmd.SetArgs([]string{"--profile", profileName, "--key", "media/file"})
	if err := cmd.Execute(); err == nil || err.Error() != "PROFILE_ERROR" {
		t.Fatalf("factory error = %v", err)
	}
}

func TestListDeleteCommandsMapOutputFailures(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	factory := func(profile.Profile) (mediaClient, error) { return &recordingMediaClient{}, nil }

	listCmd := newListCmdWithFactory(factory)
	listCmd.SetOut(failingWriter{})
	listCmd.SetArgs([]string{"--profile", profileName, "--prefix", "media/", "--limit", "1"})
	if err := listCmd.Execute(); err == nil || err.Error() != "OUTPUT_ERROR" {
		t.Fatalf("list output error = %v", err)
	}

	deleteCmd := newDeleteCmdWithFactory(factory)
	deleteCmd.SetOut(failingWriter{})
	deleteCmd.SetArgs([]string{"--profile", profileName, "--key", "media/file"})
	if err := deleteCmd.Execute(); err == nil || err.Error() != "OUTPUT_ERROR" {
		t.Fatalf("delete output error = %v", err)
	}
}

func TestDefaultFactoryMapsCanceledOperationsWithoutNetwork(t *testing.T) {
	profileName := saveCommandTestProfile(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	listCmd := newListCmd()
	listCmd.SetArgs([]string{"--profile", profileName, "--prefix", "media/", "--limit", "1"})
	if err := listCmd.ExecuteContext(ctx); err == nil || err.Error() != "OSS_LIST_ERROR" {
		t.Fatalf("list error = %v", err)
	}

	deleteCmd := newDeleteCmd()
	deleteCmd.SetArgs([]string{"--profile", profileName, "--key", "media/file"})
	if err := deleteCmd.ExecuteContext(ctx); err == nil || err.Error() != "OSS_DELETE_ERROR" {
		t.Fatalf("delete error = %v", err)
	}
}

func TestObjectSummaryTimestampIsStable(t *testing.T) {
	if got := time.Date(2026, 8, 18, 18, 0, 0, 0, time.FixedZone("CST", 8*60*60)).UTC().Format(time.RFC3339); got != "2026-08-18T10:00:00Z" {
		t.Fatal(got)
	}
}
