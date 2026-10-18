// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
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

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zeromicro/go-zero/core/logx"
)

var (
	minioCore   *minio.Core
	partsStates map[string]*uploadPartsState
)

type uploadPartsState struct {
	uploadID string
	parts    map[int32]minio.CompletePart
}

func newS3Core() (*minio.Core, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:8333"
	}
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY are required")
	}
	useSSL := false
	if value := os.Getenv("S3_USE_SSL"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("S3_USE_SSL must be true or false: %w", err)
		}
		useSSL = parsed
	}
	return minio.NewCore(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       useSSL,
		BucketLookup: minio.BucketLookupPath,
	})
}

func ensureS3Core() error {
	if minioCore != nil {
		return nil
	}
	core, err := newS3Core()
	if err != nil {
		return err
	}
	minioCore = core
	if partsStates == nil {
		partsStates = make(map[string]*uploadPartsState)
	}
	return nil
}

func SaveFilePart(creator, fileId int64, filePart int32, b []byte) error {
	if err := ensureS3Core(); err != nil {
		return err
	}

	var (
		err    error
		bucket = "echat"
		object = fmt.Sprintf("%d.%d", creator, fileId)
		state  *uploadPartsState
		ok     bool
	)

	if state, ok = partsStates[object]; !ok {
		uploadID, err := minioCore.NewMultipartUpload(context.Background(), bucket, object, minio.PutObjectOptions{})
		if err != nil {
			logx.Errorf("error - %v", err)
			return err
		}
		partsStates[object] = &uploadPartsState{
			uploadID: uploadID,
			parts:    make(map[int32]minio.CompletePart),
		}
	}
	state = partsStates[object]

	objPart, err := minioCore.PutObjectPart(
		context.Background(),
		bucket,
		object,
		state.uploadID,
		int(filePart),
		bytes.NewReader(b),
		int64(len(b)),
		"",
		"",
		nil)

	if err != nil {
		logx.Errorf("error - %v", err)
		return err
	}

	logx.Infof("objPart - %d, %s", objPart.PartNumber, objPart.ETag)

	state.parts[filePart] = minio.CompletePart{
		PartNumber: objPart.PartNumber,
		ETag:       objPart.ETag,
	}

	return nil
}

const (
	partSize = 5 * 1024 * 1024
)

func main() {
	if err := ensureS3Core(); err != nil {
		logx.Errorf("error: %v", err)
		return
	}
	var (
		err      error
		bucket   = "echat"
		object   = "1075685794330447872.-5490273994197500417.dat"
		parts    []minio.CompletePart
		uploadID string
		i        int
	)

	b, err := os.ReadFile(object)
	if err != nil {
		logx.Errorf("error: %v", err)
		return
	}

	uploadID, err = minioCore.NewMultipartUpload(context.Background(), bucket, object, minio.PutObjectOptions{})
	if err != nil {
		logx.Errorf("error: %v", err)
		return
	}

	for i = 0; i < len(b)/partSize; i++ {
		objPart, err := minioCore.PutObjectPart(
			context.Background(),
			bucket,
			object,
			uploadID,
			i+1,
			bytes.NewReader(b[i*partSize:(i+1)*partSize]),
			partSize,
			"",
			"",
			nil)

		if err != nil {
			logx.Errorf("error - %v", err)
			return
		}
		logx.Infof("%v", objPart)
		parts = append(parts, minio.CompletePart{
			PartNumber: objPart.PartNumber,
			ETag:       objPart.ETag})
	}

	lastSize := len(b) % partSize
	if lastSize > 0 {
		objPart, err := minioCore.PutObjectPart(
			context.Background(),
			bucket,
			object,
			uploadID,
			i,
			bytes.NewReader(b[i*partSize:]),
			int64(lastSize),
			"",
			"",
			nil)

		if err != nil {
			logx.Errorf("error - %v", err)
			return
		}
		logx.Infof("%v", objPart)
		parts = append(parts, minio.CompletePart{
			PartNumber: objPart.PartNumber,
			ETag:       objPart.ETag})
	}

	eTag, err := minioCore.CompleteMultipartUpload(context.Background(), bucket, object, uploadID, parts, minio.PutObjectOptions{})
	if err != nil {
		logx.Errorf("error - %v", err)
	}
	logx.Info(eTag)
}
