package dao

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
)

type testSSDBStore struct {
	kv.ExtStore
	pipeline *testSSDBPipeline
}

func (s *testSSDBStore) GetPipeline(string) (kv.Pipeline, error) {
	return s.pipeline, nil
}

type testSSDBPipeline struct {
	parts        map[string]string
	pipelineRuns int
	hgetCalls    int
}

func (p *testSSDBPipeline) Pipelined(fn func(kv.Pipeliner) error) error {
	return p.PipelinedCtx(context.Background(), fn)
}

func (p *testSSDBPipeline) PipelinedCtx(ctx context.Context, fn func(kv.Pipeliner) error) error {
	p.pipelineRuns++
	return fn(&testSSDBPipeliner{parent: p})
}

type testSSDBPipeliner struct {
	kv.Pipeliner
	parent *testSSDBPipeline
}

func (p *testSSDBPipeliner) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	p.parent.hgetCalls++
	cmd := redis.NewStringCmd(ctx, "hget", key, field)
	value, ok := p.parent.parts[field]
	if !ok {
		cmd.SetErr(redis.Nil)
	} else {
		cmd.SetVal(value)
	}
	return cmd
}

func TestSSDBReaderReadAheadAndSeek(t *testing.T) {
	pipeline := &testSSDBPipeline{parts: map[string]string{
		"0": "AB",
		"1": "CDE",
		"2": "FGH",
		"3": "IJK",
		"4": "L",
	}}
	reader := &SSDBReader{
		DfsFileInfo: &model.DfsFileInfo{
			Creator: 1, FileId: 2, FileTotalParts: 5,
			FirstFilePartSize: 2, FilePartSize: 3, LastFilePartSize: 1,
		},
		ssdb:            &testSSDBStore{pipeline: pipeline},
		cachedPartStart: -1,
	}

	readAll := func() string {
		t.Helper()
		var got strings.Builder
		buf := make([]byte, 4)
		for {
			n, err := reader.Read(buf)
			got.Write(buf[:n])
			if err == io.EOF {
				return got.String()
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	if got := readAll(); got != "ABCDEFGHIJKL" {
		t.Fatalf("read all = %q, want %q", got, "ABCDEFGHIJKL")
	}
	if pipeline.pipelineRuns != 2 || pipeline.hgetCalls != 5 {
		t.Fatalf("first read used %d pipelines and %d HGETs, want 2 pipelines and 5 HGETs", pipeline.pipelineRuns, pipeline.hgetCalls)
	}
	if pos, err := reader.Seek(7, io.SeekStart); err != nil || pos != 7 {
		t.Fatalf("Seek(7) = %d, %v; want 7, nil", pos, err)
	}
	if got := readAll(); got != "HIJKL" {
		t.Fatalf("read after seek = %q, want %q", got, "HIJKL")
	}
}
