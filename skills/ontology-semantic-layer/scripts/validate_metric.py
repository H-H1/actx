#!/usr/bin/env python3
"""校验 metrics.yaml 是否符合指标语义层规格。"""
import sys
import yaml

REQUIRED = ["name", "display_name", "business_definition", "formula",
            "numerator", "denominator", "time_window", "data_source",
            "owner", "update_frequency", "status", "version"]

def validate(path):
    errors = []
    with open(path) as f:
        data = yaml.safe_load(f)

    for m in data.get("metrics", []):
        for field in REQUIRED:
            if not m.get(field):
                errors.append(f"指标 {m.get('name', '?')} 缺 {field}")

        bd = m.get("business_definition", "")
        if bd and not all(k in bd for k in ("分子", "分母", "时间窗")):
            errors.append(f"指标 {m.get('name')} 的 business_definition 未含分子/分母/时间窗")

        for part in ("numerator", "denominator"):
            p = m.get(part, {})
            if not p.get("description") or not p.get("sql"):
                errors.append(f"指标 {m.get('name')} 的 {part} 缺 description/sql")

        tw = m.get("time_window", {})
        if not tw.get("value") or not tw.get("unit"):
            errors.append(f"指标 {m.get('name')} 的 time_window 缺 value/unit")

        ds = m.get("data_source", {})
        for field in ("table", "date_field", "member_field"):
            if not ds.get(field):
                errors.append(f"指标 {m.get('name')} 的 data_source 缺 {field}")

        for f in m.get("filters", []):
            if not f.get("description") or not f.get("sql"):
                errors.append(f"指标 {m.get('name')} 的 filter 缺 description/sql")

    if errors:
        print("❌ 校验失败：")
        for e in errors:
            print(f"  - {e}")
        return 1
    print("✅ 指标校验通过")
    return 0

if __name__ == "__main__":
    sys.exit(validate(sys.argv[1] if len(sys.argv) > 1 else "metrics.yaml"))