// Copyright (c) 2021-present, Teamgram Studio (https://teamgram.io).
// All rights reserved.
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/teamgram/teamgram-server/tools/s3support"
	"github.com/zeromicro/go-zero/core/logx"
)

var (
	s3Client    *s3.Client
	partsStates map[string]*uploadPartsState
)

type uploadPartsState struct {
	uploadID string
	parts    map[int32]types.CompletedPart
}

func newS3Client() (*s3.Client, error) {
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
	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return s3support.NewClient(s3support.Config{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		UseSSL:    useSSL,
		Region:    region,
	})
}

func ensureS3Client() error {
	if s3Client != nil {
		return nil
	}
	client, err := newS3Client()
	if err != nil {
		return err
	}
	s3Client = client
	if partsStates == nil {
		partsStates = make(map[string]*uploadPartsState)
	}
	return nil
}

func SaveFilePart(creator, fileID int64, filePart int32, b []byte) error {
	if err := ensureS3Client(); err != nil {
		return err
	}

	bucket := "echat"
	object := fmt.Sprintf("%d.%d", creator, fileID)
	state, ok := partsStates[object]
	if !ok {
		created, err := s3Client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(object),
		})
		if err != nil {
			logx.Errorf("error - %v", err)
			return err
		}
		state = &uploadPartsState{
			uploadID: aws.ToString(created.UploadId),
			parts:    make(map[int32]types.CompletedPart),
		}
		partsStates[object] = state
	}

	part, err := s3Client.UploadPart(context.Background(), &s3.UploadPartInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(object),
		UploadId:      aws.String(state.uploadID),
		PartNumber:    aws.Int32(filePart),
		Body:          bytes.NewReader(b),
		ContentLength: aws.Int64(int64(len(b))),
	})
	if err != nil {
		logx.Errorf("error - %v", err)
		return err
	}

	logx.Infof("objPart - %d, %s", filePart, aws.ToString(part.ETag))
	state.parts[filePart] = types.CompletedPart{
		PartNumber: aws.Int32(filePart),
		ETag:       part.ETag,
	}
	return nil
}

const partSize = 5 * 1024 * 1024

func main() {
	if err := ensureS3Client(); err != nil {
		logx.Errorf("error: %v", err)
		return
	}

	bucket := "echat"
	object := "1075685794330447872.-5490273994197500417.dat"
	b, err := os.ReadFile(object)
	if err != nil {
		logx.Errorf("error: %v", err)
		return
	}

	created, err := s3Client.CreateMultipartUpload(context.Background(), &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(object),
	})
	if err != nil {
		logx.Errorf("error: %v", err)
		return
	}
	uploadID := aws.ToString(created.UploadId)
	parts := make([]types.CompletedPart, 0, (len(b)+partSize-1)/partSize)
	for partNumber, offset := int32(1), 0; offset < len(b); partNumber, offset = partNumber+1, offset+partSize {
		end := offset + partSize
		if end > len(b) {
			end = len(b)
		}
		part, err := s3Client.UploadPart(context.Background(), &s3.UploadPartInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(object),
			UploadId:      aws.String(uploadID),
			PartNumber:    aws.Int32(partNumber),
			Body:          bytes.NewReader(b[offset:end]),
			ContentLength: aws.Int64(int64(end - offset)),
		})
		if err != nil {
			logx.Errorf("error - %v", err)
			return
		}
		logx.Infof("part=%d etag=%s", partNumber, aws.ToString(part.ETag))
		parts = append(parts, types.CompletedPart{PartNumber: aws.Int32(partNumber), ETag: part.ETag})
	}

	completed, err := s3Client.CompleteMultipartUpload(context.Background(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(object),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	})
	if err != nil {
		logx.Errorf("error - %v", err)
		return
	}
	logx.Info(aws.ToString(completed.ETag))
}
