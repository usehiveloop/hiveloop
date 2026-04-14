#!/usr/bin/env python3
"""
Benchmark embedding accuracy for code review use cases.

Tests whether the embedding model can correctly identify:
1. Similar functions (pattern matching)
2. Related functions (same domain/feature)
3. Functions that should be reviewed together (co-change candidates)
4. Type/interface implementations

Each test case has a query and expected results (ground truth).
We measure: Hit@1, Hit@5, Hit@10, MRR (Mean Reciprocal Rank).
"""

import json
import struct
import sqlite3
import sys
import time
import os

import sqlite_vec
from sentence_transformers import SentenceTransformer


def serialize_f32(vector):
    return struct.pack(f"{len(vector)}f", *vector)


def search(database, model, query_text, limit=10, language=None):
    """Search for similar symbols."""
    embedding = model.encode(
        [f"Represent this query for searching relevant code: {query_text}"],
        normalize_embeddings=True
    )[0]

    vec_query = serialize_f32(embedding.tolist())
    fetch_limit = limit * 5 if language else limit

    rows = database.execute("""
        SELECT vec_symbols.rowid, vec_symbols.distance
        FROM vec_symbols
        WHERE embedding MATCH ?
        ORDER BY distance
        LIMIT ?
    """, (vec_query, fetch_limit)).fetchall()

    results = []
    for row in rows:
        symbol = database.execute(
            "SELECT name, file_path, node_type, language, start_line, end_line FROM symbols WHERE id = ?",
            (row[0],)
        ).fetchone()
        if symbol is None:
            continue
        if language and symbol[3] != language:
            continue
        results.append({
            "name": symbol[0],
            "file_path": symbol[1],
            "node_type": symbol[2],
            "language": symbol[3],
            "distance": row[1],
            "similarity": round(1 - row[1], 4),
        })
        if len(results) >= limit:
            break

    return results


def run_test_case(database, model, test_case):
    """Run a single test case and return metrics."""
    query = test_case["query"]
    expected_names = set(test_case["expected_names"])
    category = test_case["category"]
    language = test_case.get("language")

    results = search(database, model, query, limit=10, language=language)
    result_names = [result["name"] for result in results]

    # Hit@k: did any expected name appear in top k?
    hit_at_1 = 1 if len(result_names) > 0 and result_names[0] in expected_names else 0
    hit_at_3 = 1 if any(name in expected_names for name in result_names[:3]) else 0
    hit_at_5 = 1 if any(name in expected_names for name in result_names[:5]) else 0
    hit_at_10 = 1 if any(name in expected_names for name in result_names[:10]) else 0

    # MRR: reciprocal rank of first correct result
    mrr = 0
    for idx, name in enumerate(result_names):
        if name in expected_names:
            mrr = 1.0 / (idx + 1)
            break

    # Count how many expected names were found in top 10
    found = [name for name in result_names if name in expected_names]
    recall_at_10 = len(found) / len(expected_names) if expected_names else 0

    return {
        "query": query,
        "category": category,
        "expected": list(expected_names),
        "found": found,
        "top_5_results": [{"name": r["name"], "file": r["file_path"], "similarity": r["similarity"]} for r in results[:5]],
        "hit_at_1": hit_at_1,
        "hit_at_3": hit_at_3,
        "hit_at_5": hit_at_5,
        "hit_at_10": hit_at_10,
        "mrr": mrr,
        "recall_at_10": recall_at_10,
    }


def main():
    db_path = sys.argv[1] if len(sys.argv) > 1 else "/workspace/vectors.db"
    model_path = sys.argv[2] if len(sys.argv) > 2 else "nomic-ai/CodeRankEmbed"
    test_file = sys.argv[3] if len(sys.argv) > 3 else None

    print("Loading model...")
    model = SentenceTransformer(model_path, trust_remote_code=True)

    print("Connecting to database...")
    database = sqlite3.connect(db_path)
    database.enable_load_extension(True)
    sqlite_vec.load(database)
    database.enable_load_extension(False)

    total = database.execute("SELECT COUNT(*) FROM symbols").fetchone()[0]
    print(f"Database has {total} symbols")

    # Load test cases from file or use built-in ones
    if test_file and os.path.exists(test_file):
        with open(test_file) as file_handle:
            test_cases = json.load(file_handle)
    else:
        # Auto-generate test cases from the database
        print("\nNo test file provided. Running auto-discovery benchmark...")
        test_cases = generate_auto_tests(database)

    if not test_cases:
        print("No test cases to run.")
        return

    # Run all tests
    print(f"\nRunning {len(test_cases)} test cases...\n")
    all_results = []
    start = time.time()

    for test_case in test_cases:
        result = run_test_case(database, model, test_case)
        all_results.append(result)

        status = "PASS" if result["hit_at_5"] else "MISS"
        print(f"  [{status}] {result['category']:20s} | {result['query'][:50]:50s} | MRR={result['mrr']:.2f} | Found: {result['found']}")

    elapsed = time.time() - start

    # Aggregate metrics
    total_tests = len(all_results)
    avg_hit1 = sum(r["hit_at_1"] for r in all_results) / total_tests
    avg_hit3 = sum(r["hit_at_3"] for r in all_results) / total_tests
    avg_hit5 = sum(r["hit_at_5"] for r in all_results) / total_tests
    avg_hit10 = sum(r["hit_at_10"] for r in all_results) / total_tests
    avg_mrr = sum(r["mrr"] for r in all_results) / total_tests
    avg_recall = sum(r["recall_at_10"] for r in all_results) / total_tests

    # By category
    categories = {}
    for result in all_results:
        cat = result["category"]
        if cat not in categories:
            categories[cat] = []
        categories[cat].append(result)

    print(f"\n{'='*70}")
    print(f"BENCHMARK RESULTS ({total_tests} test cases, {elapsed:.1f}s)")
    print(f"{'='*70}")
    print(f"  Hit@1:       {avg_hit1:.1%}")
    print(f"  Hit@3:       {avg_hit3:.1%}")
    print(f"  Hit@5:       {avg_hit5:.1%}")
    print(f"  Hit@10:      {avg_hit10:.1%}")
    print(f"  MRR:         {avg_mrr:.3f}")
    print(f"  Recall@10:   {avg_recall:.1%}")

    print(f"\nBy category:")
    for cat, cat_results in sorted(categories.items()):
        cat_mrr = sum(r["mrr"] for r in cat_results) / len(cat_results)
        cat_hit5 = sum(r["hit_at_5"] for r in cat_results) / len(cat_results)
        print(f"  {cat:25s} | MRR={cat_mrr:.3f} | Hit@5={cat_hit5:.1%} | n={len(cat_results)}")

    print(f"{'='*70}")

    # Save detailed results
    output_path = db_path.replace(".db", "_benchmark.json")
    with open(output_path, "w") as output_file:
        json.dump({
            "summary": {
                "total_tests": total_tests,
                "hit_at_1": avg_hit1,
                "hit_at_3": avg_hit3,
                "hit_at_5": avg_hit5,
                "hit_at_10": avg_hit10,
                "mrr": avg_mrr,
                "recall_at_10": avg_recall,
                "elapsed_s": elapsed,
            },
            "by_category": {
                cat: {
                    "mrr": sum(r["mrr"] for r in cat_results) / len(cat_results),
                    "hit_at_5": sum(r["hit_at_5"] for r in cat_results) / len(cat_results),
                    "count": len(cat_results),
                }
                for cat, cat_results in categories.items()
            },
            "details": all_results,
        }, output_file, indent=2)
    print(f"\nDetailed results saved to {output_path}")


def generate_auto_tests(database):
    """Auto-generate test cases by finding natural clusters in the codebase."""
    test_cases = []

    # Test 1: Find functions with similar names (pattern matching)
    # Look for groups of functions with common prefixes/suffixes
    name_groups = database.execute("""
        SELECT name, file_path, node_type FROM symbols
        WHERE node_type IN ('function_declaration', 'method_declaration', 'function_definition', 'method_definition')
        ORDER BY name
    """).fetchall()

    # Group by common suffixes (Handler, Middleware, Service, etc.)
    suffix_groups = {}
    for row in name_groups:
        name = row[0]
        for suffix in ["Handler", "Middleware", "Service", "Controller", "Repository", "Provider", "Factory", "Manager", "Helper", "Util"]:
            if name.endswith(suffix) and len(name) > len(suffix):
                if suffix not in suffix_groups:
                    suffix_groups[suffix] = []
                suffix_groups[suffix].append({"name": name, "file": row[1]})

    for suffix, group in suffix_groups.items():
        if len(group) >= 3:
            # Use first function as query, expect to find others
            query_func = group[0]
            expected = [func["name"] for func in group[1:4]]  # expect top 3 others
            test_cases.append({
                "query": f"func {query_func['name']}",
                "expected_names": expected,
                "category": f"pattern:{suffix}",
            })
            if len(test_cases) >= 5:
                break

    # Test 2: Find functions in the same file (co-location)
    file_groups = database.execute("""
        SELECT file_path, GROUP_CONCAT(name, '|||') as names
        FROM symbols
        WHERE node_type IN ('function_declaration', 'method_declaration', 'function_definition', 'method_definition')
        GROUP BY file_path
        HAVING COUNT(*) BETWEEN 3 AND 15
        ORDER BY RANDOM()
        LIMIT 5
    """).fetchall()

    for row in file_groups:
        names = row[1].split("|||")
        if len(names) >= 3:
            query_name = names[0]
            expected = names[1:4]
            test_cases.append({
                "query": f"functions in the same module as {query_name}",
                "expected_names": expected,
                "category": "co-location",
            })

    # Test 3: Find struct/class and its methods
    structs = database.execute("""
        SELECT name, file_path FROM symbols
        WHERE node_type IN ('type_declaration', 'class_declaration', 'class_definition')
        ORDER BY RANDOM()
        LIMIT 5
    """).fetchall()

    for struct_row in structs:
        struct_name = struct_row[0]
        struct_file = struct_row[1]
        # Find methods in the same file
        methods = database.execute("""
            SELECT name FROM symbols
            WHERE file_path = ? AND node_type IN ('method_declaration', 'method_definition')
            LIMIT 5
        """, (struct_file,)).fetchall()
        if methods:
            test_cases.append({
                "query": f"methods of {struct_name}",
                "expected_names": [m[0] for m in methods],
                "category": "struct-methods",
            })

    # Test 4: Natural language queries for common patterns
    nl_queries = [
        {"query": "HTTP request handler", "expected_prefix": "Handle", "category": "nl:handler"},
        {"query": "database migration", "expected_prefix": "Migrate", "category": "nl:migration"},
        {"query": "authentication middleware", "expected_suffix": "Auth", "category": "nl:auth"},
        {"query": "error handling", "expected_suffix": "Error", "category": "nl:error"},
        {"query": "test helper function", "expected_prefix": "Test", "category": "nl:test"},
    ]

    for nlq in nl_queries:
        if "expected_prefix" in nlq:
            matches = database.execute(
                "SELECT name FROM symbols WHERE name LIKE ? LIMIT 5",
                (f"{nlq['expected_prefix']}%",)
            ).fetchall()
        else:
            matches = database.execute(
                "SELECT name FROM symbols WHERE name LIKE ? LIMIT 5",
                (f"%{nlq['expected_suffix']}%",)
            ).fetchall()

        if matches:
            test_cases.append({
                "query": nlq["query"],
                "expected_names": [m[0] for m in matches],
                "category": nlq["category"],
            })

    return test_cases


if __name__ == "__main__":
    main()
