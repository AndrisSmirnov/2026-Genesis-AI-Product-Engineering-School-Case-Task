"""Generate reference values for the Go stats package.

Run once (not part of the skill runtime):
    python3 -m venv .venv && .venv/bin/pip install scipy pymannkendall numpy
    .venv/bin/python dev/gen_fixtures.py

Writes scripts/internal/stats/testdata/reference.json. The Go tests compare
their own implementation against these numbers (scipy 1.13 / pymannkendall 1.4).
"""
import json
import pathlib

import numpy as np
import pymannkendall as mk
from scipy import stats

rng = np.random.default_rng(42)
months = np.arange(36)

series = {
    "trend_up_noisy": (100 * 1.015 ** months * (1 + 0.05 * rng.standard_normal(36))).round(3),
    "flat_noise": (100 + 5 * rng.standard_normal(36)).round(3),
    "seasonal_up": (100 + 0.8 * months + 15 * np.sin(2 * np.pi * months / 12) + 3 * rng.standard_normal(36)).round(3),
    "with_ties": np.array([5, 3, 5, 5, 7, 3, 8, 9, 9, 9, 10, 4, 12, 12, 11, 15, 15, 14, 16, 18, 18, 20, 21, 19], dtype=float),
    "short_down": np.array([30, 28, 29, 25, 24, 26, 21, 20, 19, 22], dtype=float),
}

out = {"cases": []}
for name, y in series.items():
    x = np.arange(len(y), dtype=float)
    ts = stats.theilslopes(y, x, alpha=0.90, method="joint")
    orig = mk.original_test(y)
    case = {
        "name": name,
        "y": [float(v) for v in y],
        "median": float(np.median(y)),
        "mad": float(stats.median_abs_deviation(y, scale=1.0)),
        "q05": float(np.quantile(y, 0.05)),
        "q95": float(np.quantile(y, 0.95)),
        "theil_sen": {"slope": float(ts.slope), "intercept": float(ts.intercept),
                      "low90": float(ts.low_slope), "high90": float(ts.high_slope)},
        "mk": {"s": float(orig.s), "var_s": float(orig.var_s), "z": float(orig.z), "p": float(orig.p)},
    }
    if len(y) >= 24:
        sea = mk.seasonal_test(y, period=12)
        case["smk"] = {"s": float(sea.s), "var_s": float(sea.var_s), "z": float(sea.z), "p": float(sea.p)}
    out["cases"].append(case)

dst = pathlib.Path(__file__).resolve().parents[1] / "scripts/internal/stats/testdata/reference.json"
dst.parent.mkdir(parents=True, exist_ok=True)
dst.write_text(json.dumps(out, indent=1))
print("wrote", dst)
