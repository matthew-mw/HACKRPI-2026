import importlib.util
from pathlib import Path
import unittest
from urllib.parse import urlsplit, unquote

spec = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare-deploy-env.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class CredentialsTest(unittest.TestCase):
    def test_literal_special_characters(self):
        password = 'a@:/?#%$ &+="\\é'
        output = module.prepare('DATABASE_URL=postgresql://user@db.example:5432/tsdb?sslmode=require\nDATABASE_PASSWORD=' + password)
        url = output.strip().split('DATABASE_URL=')[1]
        self.assertEqual(unquote(urlsplit(url).password), password)
        self.assertNotIn('DATABASE_PASSWORD', output)
        self.assertNotIn('$', output)

    def test_separate_password_repairs_url(self):
        output = module.prepare('DATABASE_URL=postgresql://user:bad/#?@db.example/tsdb?sslmode=require\nDATABASE_PASSWORD=new%40@password')
        self.assertEqual(unquote(urlsplit(output.strip().split('=', 1)[1]).password), 'new%40@password')

    def test_encoded_url_unchanged(self):
        source = 'DATABASE_URL=postgresql://user:p%40ss@db.example/tsdb?sslmode=require\n'
        self.assertEqual(module.prepare(source), source)

    def test_passwordless_url_passes_through(self):
        source = 'DATABASE_URL=postgresql://user@db.example/tsdb?sslmode=require\n'
        self.assertEqual(module.prepare(source), source)

    def test_insecure_tls(self):
        for url in ['postgresql://user:pass@db.example/tsdb?sslmode=disable']:
            with self.assertRaises(ValueError):
                module.prepare('DATABASE_URL=' + url)

if __name__ == '__main__':
    unittest.main()
