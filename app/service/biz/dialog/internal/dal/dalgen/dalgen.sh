#!/bin/bash

dalgen3 --xml=$1 --db=teamgram --go2=github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject

gofmt -w ../dataobject/*.go
