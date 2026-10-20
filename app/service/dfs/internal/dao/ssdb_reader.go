// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type stringResult interface {
	Result() (string, error)
}

type SSDBReader struct {
	*model.DfsFileInfo
	ssdb            kv.ExtStore
	i               int64 // current reading index
	cachedPartStart int
	cachedParts     []string
}

const ssdbReadAheadParts = 4

func (d *Dao) NewSSDBReader(fileInfo *model.DfsFileInfo) *SSDBReader {
	return &SSDBReader{
		DfsFileInfo:     fileInfo,
		ssdb:            d.ssdb,
		i:               0,
		cachedPartStart: -1,
	}
}

func (r *SSDBReader) getPartIdx() int {
	if r.i < int64(r.FirstFilePartSize) {
		return 0
	}
	return 1 + (int(r.i)-r.FirstFilePartSize)/r.FilePartSize
}

func (r *SSDBReader) getPartOffset() int {
	if r.i < int64(r.FirstFilePartSize) {
		return int(r.i)
	} else {
		return (int(r.i) - r.FirstFilePartSize) % r.FilePartSize
	}
}

func (r *SSDBReader) Read(p []byte) (n int, err error) {
	if r.i >= r.DfsFileInfo.GetFileSize() {
		return 0, io.EOF
	}

	l := len(p)
	if l == 0 {
		return
	}

	for n < l && r.i < r.DfsFileInfo.GetFileSize() {
		idx := r.getPartIdx()
		if err = r.loadPartWindow(context.Background(), idx); err != nil {
			return n, err
		}

		partData := r.cachedParts[idx-r.cachedPartStart]
		offset := r.getPartOffset()
		if offset >= len(partData) {
			return n, io.ErrUnexpectedEOF
		}
		available := len(partData) - offset
		readSize := l - n
		if readSize > available {
			readSize = available
		}
		copy(p[n:n+readSize], partData[offset:offset+readSize])
		n += readSize
		r.i += int64(readSize)
	}

	return n, nil
}

// Seek implements the io.Seeker interface.
func (r *SSDBReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.i + offset
	case io.SeekEnd:
		abs = r.DfsFileInfo.GetFileSize() + offset
	default:
		return 0, fmt.Errorf("ssdb.Reader.Seek: invalid whence")
	}
	if abs < 0 {
		return 0, fmt.Errorf("ssdb.Reader.Seek: negative position")
	}
	r.i = abs
	r.cachedPartStart = -1
	r.cachedParts = nil
	return abs, nil
}

func (r *SSDBReader) loadPartWindow(ctx context.Context, filePart int) error {
	if filePart >= r.cachedPartStart && filePart < r.cachedPartStart+len(r.cachedParts) {
		return nil
	}
	start := filePart - filePart%ssdbReadAheadParts
	end := start + ssdbReadAheadParts
	if end > r.FileTotalParts {
		end = r.FileTotalParts
	}
	if start < 0 || start >= end {
		return io.ErrUnexpectedEOF
	}
	k := getFileKey(r.Creator, r.FileId)
	pipeline, err := r.ssdb.GetPipeline(k)
	if err != nil {
		logx.WithContext(ctx).Errorf("get SSDB pipeline %s error(%v)", k, err)
		return err
	}
	results := make([]stringResult, end-start)
	err = pipeline.PipelinedCtx(ctx, func(pipe kv.Pipeliner) error {
		for i := start; i < end; i++ {
			results[i-start] = pipe.HGet(ctx, k, strconv.Itoa(i))
		}
		return nil
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("pipeline HGET %s error(%v)", k, err)
		return err
	}
	parts := make([]string, len(results))
	for i, result := range results {
		value, resultErr := result.Result()
		if resultErr != nil {
			logx.WithContext(ctx).Errorf("pipeline HGET %s, %d error(%v)", k, start+i, resultErr)
			return resultErr
		}
		parts[i] = value
	}
	r.cachedPartStart = start
	r.cachedParts = parts

	return nil
}

func (r *SSDBReader) ReadAll(ctx context.Context) ([]byte, error) {

	var (
		bytes []byte
		k     = getFileKey(r.Creator, r.FileId)
	)
	if size := r.GetFileSize(); size > 0 && size <= int64(^uint(0)>>1) {
		bytes = make([]byte, 0, int(size))
	}

	pipeline, err := r.ssdb.GetPipeline(k)
	if err != nil {
		logx.WithContext(ctx).Errorf("get SSDB pipeline %s error(%v)", k, err)
		return nil, err
	}
	results := make([]stringResult, r.FileTotalParts)
	err = pipeline.PipelinedCtx(ctx, func(pipe kv.Pipeliner) error {
		for i := 0; i < r.FileTotalParts; i++ {
			results[i] = pipe.HGet(ctx, k, strconv.Itoa(i))
		}
		return nil
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("pipeline HGET %s error(%v)", k, err)
		return nil, err
	}

	for i, result := range results {
		bBuf, err := result.Result()
		if err != nil {
			logx.WithContext(ctx).Errorf("pipeline HGET %s, %d error(%v)", k, i, err)
			return nil, err
		}
		bytes = append(bytes, bBuf...)
	}

	return bytes, nil
}
