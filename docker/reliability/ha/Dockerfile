FROM postgres:18
RUN apt-get update && apt-get install -y --no-install-recommends python3-venv \
    && python3 -m venv /opt/patroni \
    && /opt/patroni/bin/pip install --no-cache-dir 'patroni[psycopg3,etcd3]==4.1.5' \
    && rm -rf /var/lib/apt/lists/*
COPY start.py /opt/start.py
ENTRYPOINT ["python3", "/opt/start.py"]
