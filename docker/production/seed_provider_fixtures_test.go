package production_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeedProviderFixturesContract(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	script := filepath.Join(filepath.Dir(sourceFile), "seed-provider-fixtures.sh")
	if output, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("seed-provider-fixtures.sh is not valid bash: %v\n%s", err, output)
	}
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read seed-provider-fixtures.sh: %v", err)
	}
	contents := string(body)
	for _, contract := range []string{
		"business_connections_json=${BUSINESS_CONNECTIONS_JSON:-}",
		"gift_ownerships_json=${GIFT_OWNERSHIPS_JSON:-}",
		"stars_credits_json=${STARS_CREDITS_JSON:-}",
		"business_bot_operations_json=${BUSINESS_BOT_OPERATIONS_JSON:-}",
		"business_payment_operations_json=${BUSINESS_PAYMENT_OPERATIONS_JSON:-}",
		"--variable=business_connections_enabled=",
		"--variable=gift_ownerships_enabled=",
		"--variable=stars_credits_enabled=",
		"--variable=business_bot_operations_enabled=",
		"--variable=business_payment_operations_enabled=",
		"\\if :business_connections_enabled",
		"\\if :gift_ownerships_enabled",
		"\\if :stars_credits_enabled",
		"\\if :business_bot_operations_enabled",
		"\\if :business_payment_operations_enabled",
		"INSERT INTO apifull_business_connection",
		"INSERT INTO apifull_gift",
		"INSERT INTO apifull_star_tx",
		"INSERT INTO apifull_stars",
		"INSERT INTO teamgram_business_bot_operation",
		"INSERT INTO teamgram_business_payment_operation",
		"jsonb_to_recordset(:'business_connections_json'::jsonb)",
		"jsonb_to_recordset(:'gift_ownerships_json'::jsonb)",
		"jsonb_to_recordset(:'stars_credits_json'::jsonb)",
		"jsonb_to_recordset(:'business_bot_operations_json'::jsonb)",
		"jsonb_to_recordset(:'business_payment_operations_json'::jsonb)",
	} {
		if !strings.Contains(contents, contract) {
			t.Errorf("seed script is missing contract %q", contract)
		}
	}
}
