"""Unit tests for M0 identity verification. No database connections or writes."""
import unittest
from m0.run_m0 import verify_distinct

def url(project, host="aws-0-eu-west-1.pooler.supabase.com"):
    return f"postgresql://postgres.{project}:secret@{host}:5432/postgres"

class VerifyDistinctTests(unittest.TestCase):
    def test_cloned_cluster_identifiers_do_not_override_distinct_supabase_routes(self):
        calls = []
        def sql(connection, query):
            calls.append((connection, query))
            return "postgres"
        result = verify_distinct(url("hfvjpsniwcvkiulbdpbc"), url("kikurisvmnqjpebqbtvx"), sql)
        self.assertIn("distinct Supabase", result)
        self.assertEqual(len(calls), 2)

    def test_same_project_ref_rejected_even_with_different_hosts(self):
        project = "hfvjpsniwcvkiulbdpbc"
        with self.assertRaisesRegex(AssertionError, "references match"):
            verify_distinct(url(project), url(project, "aws-0-eu-west-2.pooler.supabase.com"), lambda *_: "postgres")

    def test_mixed_pooler_and_direct_rejected(self):
        with self.assertRaisesRegex(AssertionError, "Both connections"):
            verify_distinct(url("hfvjpsniwcvkiulbdpbc"),
                            "postgresql://postgres:secret@db.example.com:5432/postgres",
                            lambda *_: "postgres")

    def test_invalid_pooler_project_rejected(self):
        with self.assertRaisesRegex(AssertionError, "Invalid Supabase"):
            verify_distinct(url("short"), url("kikurisvmnqjpebqbtvx"), lambda *_: "postgres")

    def test_non_pooler_matching_cluster_rejected(self):
        direct = "postgresql://postgres:secret@db.example.com:5432/postgres"
        with self.assertRaisesRegex(AssertionError, "same database identity"):
            verify_distinct(direct, direct.replace("db.example.com", "db2.example.com"), lambda *_: "123:5")

if __name__ == "__main__":
    unittest.main()
