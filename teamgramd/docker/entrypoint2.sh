#!/bin/bash
# if [ ! -n "$TEAMGRAM_HOST" ]; then
#   echo ">>> Plase set environment variable TEAMGRAM_HOST to your own server IP. <<<"
#   exit 1
# fi

export TEAMGRAM_HOST=${TEAMGRAM_HOST:-"0.0.0.0"}
export ETCD_URL=${ETCD_URL:-"etcd:2379"}
export REDIS_HOST=${REDIS_HOST:-"redis:6379"}
export KAFKA_HOST=${KAFKA_HOST:-"kafka:9092"}
export POSTGRES_URI=${POSTGRES_URI:-"postgres://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable"}
export S3_ENDPOINT=${S3_ENDPOINT:-"localhost:8333"}
export S3_ACCESS_KEY=${S3_ACCESS_KEY:-"seaweedfs"}
export S3_SECRET_KEY=${S3_SECRET_KEY:-"seaweedfs-secret"}
export S3_USE_SSL=${S3_USE_SSL:-"false"}
export S3_REGION=${S3_REGION:-"us-east-1"}
#export MTZ=${MTZ:-"Asia%2FTehran"}

# create configs from config templates.
createConfigs() {
  CONFIG_TARGET_DIR=/Users/wubenqi/go/src/github.com/teamgram/teamgram-server/teamgramd/etc2
  CONFIG_TEMPLATES_DIR=/Users/wubenqi/go/src/github.com/teamgram/teamgram-server/teamgramd/etc
  for file in `ls $CONFIG_TEMPLATES_DIR`; do
    cat $CONFIG_TEMPLATES_DIR/$file \
      | sed 's#ListenOn: 127.0.0.1#ListenOn: '"$TEAMGRAM_HOST"'#g' \
      | sed "s#127.0.0.1:2379#$ETCD_URL#g" \
      | sed "s#127.0.0.1:6379#$REDIS_HOST#g" \
      | sed "s#localhost:6379#$REDIS_HOST#g" \
      | sed "s#postgres://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable#$POSTGRES_URI#g" \
      | sed 's#AccessKeyID: seaweedfs#AccessKeyID: '"$S3_ACCESS_KEY"'#g' \
      | sed 's#SecretAccessKey: seaweedfs-secret#SecretAccessKey: '"$S3_SECRET_KEY"'#g' \
      | sed 's#UseSSL: false#UseSSL: '"$S3_USE_SSL"'#g' \
      | sed 's#Region: us-east-1#Region: '"$S3_REGION"'#g' \
      | sed "s#localhost:8333#$S3_ENDPOINT#g" \
      | sed "s#127.0.0.1:9092#$KAFKA_HOST#g" \
      | cat > $CONFIG_TARGET_DIR/$file
  done
}

createConfigs

#cd /app/bin
#./runall2.sh
#
#tail -f /dev/null
