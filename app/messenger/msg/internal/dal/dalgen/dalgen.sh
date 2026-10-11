#!/bin/bash

dalgen3 --xml=$1 --db=teamgram --go2=github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject

gofmt -w ../dataobject/*.go
