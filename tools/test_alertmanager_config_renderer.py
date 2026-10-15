import importlib.util
from pathlib import Path
import tempfile
import unittest


MODULE_PATH = Path(__file__).resolve().parents[1] / "docker/production/render_alertmanager_config.py"
SPEC = importlib.util.spec_from_file_location("render_alertmanager_config", MODULE_PATH)
renderer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(renderer)


class AlertmanagerConfigRendererTests(unittest.TestCase):
    def test_renders_single_quoted_webhook_as_a_yaml_scalar(self):
        template = "receivers:\n  - name: oncall\n    webhook_configs:\n      - " + renderer.PLACEHOLDER + "\n"
        rendered = renderer.render(template, "https://alerts.example.invalid/hooks/a'b")
        self.assertIn("url: 'https://alerts.example.invalid/hooks/a''b'", rendered)
        self.assertNotIn("${TEAMGRAM_ALERT_WEBHOOK_URL}", rendered)

    def test_rejects_non_https_and_multiline_urls(self):
        for url in ("http://alerts.example.invalid/hook", "https://alerts.example.invalid/hook\nreceiver: null"):
            with self.subTest(url=url), self.assertRaises(ValueError):
                renderer.render("x\n" + renderer.PLACEHOLDER + "\n", url)

    def test_writes_private_atomic_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            template = root / "template.yml"
            output = root / "nested/alertmanager.yml"
            template.write_text(renderer.PLACEHOLDER + "\n", encoding="utf-8")
            renderer.write_config(template, output, "https://alerts.example.invalid/token")
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            self.assertEqual(output.read_text(encoding="utf-8"), "url: 'https://alerts.example.invalid/token'\n")
            self.assertEqual(list(output.parent.glob(".alertmanager.yml.*")), [])


if __name__ == "__main__":
    unittest.main()
