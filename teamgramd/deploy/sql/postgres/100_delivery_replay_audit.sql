-- Explicit dead-letter replay is audited and never discards the stored payload.
CREATE TABLE IF NOT EXISTS msg_delivery_outbox_replay_audit (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    queue_name TEXT NOT NULL CHECK (queue_name IN ('inbox', 'state')),
    outbox_id BIGINT NOT NULL,
    operator_name TEXT NOT NULL,
    ticket_ref TEXT NOT NULL,
    reason TEXT NOT NULL,
    attempts_before INTEGER NOT NULL,
    terminal_reason TEXT NOT NULL,
    payload_md5 TEXT NOT NULL,
    replayed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS msg_delivery_replay_audit_item_idx
    ON msg_delivery_outbox_replay_audit (queue_name, outbox_id, replayed_at DESC);

CREATE OR REPLACE FUNCTION public.replay_teamgram_delivery(
    p_queue_name TEXT,
    p_outbox_id BIGINT,
    p_operator_name TEXT,
    p_ticket_ref TEXT,
    p_reason TEXT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $function$
DECLARE
    v_payload JSONB;
    v_attempts INTEGER;
    v_terminal_reason TEXT;
BEGIN
    IF p_queue_name IS NULL OR p_queue_name NOT IN ('inbox', 'state')
       OR p_outbox_id IS NULL OR p_outbox_id <= 0
       OR length(trim(p_operator_name)) = 0 OR length(trim(p_operator_name)) > 128
       OR length(trim(p_ticket_ref)) = 0 OR length(trim(p_ticket_ref)) > 128
       OR length(trim(p_reason)) = 0 OR length(trim(p_reason)) > 1024 THEN
        RAISE EXCEPTION 'queue, outbox id, operator, ticket and reason are required';
    END IF;

    IF p_queue_name = 'inbox' THEN
        SELECT payload, attempts, terminal_reason
        INTO v_payload, v_attempts, v_terminal_reason
        FROM public.msg_inbox_delivery_outbox
        WHERE id = p_outbox_id AND state = 2
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'dead-lettered inbox intent % not found', p_outbox_id USING ERRCODE = 'P0002';
        END IF;
        INSERT INTO public.msg_delivery_outbox_replay_audit
            (queue_name, outbox_id, operator_name, ticket_ref, reason, attempts_before, terminal_reason, payload_md5)
        VALUES (p_queue_name, p_outbox_id, trim(p_operator_name), trim(p_ticket_ref), trim(p_reason),
                v_attempts, v_terminal_reason, md5(v_payload::text));
        UPDATE public.msg_inbox_delivery_outbox
        SET state = 0, attempts = 0, available_at = now(), lease_until = NULL, claim_token = NULL,
            last_error = '', terminal_reason = '', dead_lettered_at = NULL
        WHERE id = p_outbox_id AND state = 2;
    ELSE
        SELECT payload, attempts, terminal_reason
        INTO v_payload, v_attempts, v_terminal_reason
        FROM public.msg_state_delivery_outbox
        WHERE id = p_outbox_id AND state = 2
        FOR UPDATE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'dead-lettered state intent % not found', p_outbox_id USING ERRCODE = 'P0002';
        END IF;
        INSERT INTO public.msg_delivery_outbox_replay_audit
            (queue_name, outbox_id, operator_name, ticket_ref, reason, attempts_before, terminal_reason, payload_md5)
        VALUES (p_queue_name, p_outbox_id, trim(p_operator_name), trim(p_ticket_ref), trim(p_reason),
                v_attempts, v_terminal_reason, md5(v_payload::text));
        UPDATE public.msg_state_delivery_outbox
        SET state = 0, attempts = 0, available_at = now(), lease_until = NULL, claim_token = NULL,
            last_error = '', terminal_reason = '', dead_lettered_at = NULL
        WHERE id = p_outbox_id AND state = 2;
    END IF;
END
$function$;

REVOKE ALL ON FUNCTION public.replay_teamgram_delivery(TEXT, BIGINT, TEXT, TEXT, TEXT) FROM PUBLIC;
