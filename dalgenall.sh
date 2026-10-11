#!/usr/bin/env bash

PWD=`pwd`
TEAMGRAMAPP=${PWD}"/app"
echo ${PWD}

for dalgen_dir in \
  "${TEAMGRAMAPP}/messenger/msg/internal/dal/dalgen" \
  "${TEAMGRAMAPP}/service/biz/chat/internal/dal/dalgen" \
  "${TEAMGRAMAPP}/service/biz/message/internal/dal/dalgen" \
  "${TEAMGRAMAPP}/service/biz/user/internal/dal/dalgen" \
  "${TEAMGRAMAPP}/service/biz/dialog/internal/dal/dalgen" \
  "${TEAMGRAMAPP}/service/media/internal/dal/dalgen"; do
  if [ -x "${dalgen_dir}/dalgen_all.sh" ]; then
    (cd "${dalgen_dir}" && ./dalgen_all.sh)
  fi
done

