#!/usr/bin/env python3
"""校验 ontology.yaml 是否符合本体规格。"""
import sys
import yaml

VALID_TYPES = {"string", "decimal", "datetime", "enum", "boolean"}
VALID_CARDINALITY = {"one_to_one", "one_to_many", "many_to_one", "many_to_many"}

def validate(path):
    errors = []
    with open(path) as f:
        data = yaml.safe_load(f)

    objects = data.get("objects", [])
    links = data.get("links", [])
    actions = data.get("actions", [])

    obj_names = set()
    for o in objects:
        for field in ("name", "display_name", "primary_key"):
            if not o.get(field):
                errors.append(f"Object 缺 {field}: {o}")
        obj_names.add(o.get("name"))

        for p in o.get("properties", []):
            if not p.get("name") or not p.get("type"):
                errors.append(f"Object {o['name']} 的 property 缺 name/type: {p}")
            elif p["type"] not in VALID_TYPES:
                errors.append(f"Object {o['name']} 的 property {p['name']} 类型非法: {p['type']}")
            elif p["type"] == "enum" and not p.get("values"):
                errors.append(f"Object {o['name']} 的 enum property {p['name']} 缺 values")

    for l in links:
        for field in ("name", "display_name", "from", "to", "cardinality"):
            if not l.get(field):
                errors.append(f"Link 缺 {field}: {l}")
        if l.get("cardinality") and l["cardinality"] not in VALID_CARDINALITY:
            errors.append(f"Link {l['name']} cardinality 非法: {l['cardinality']}")
        for end in ("from", "to"):
            if l.get(end) and l[end] not in obj_names:
                errors.append(f"Link {l['name']} 的 {end} 引用了未定义的 Object: {l[end]}")

    for a in actions:
        for field in ("name", "display_name", "subject", "object"):
            if not a.get(field):
                errors.append(f"Action 缺 {field}: {a}")
        for end in ("subject", "object"):
            if a.get(end) and a[end] not in obj_names:
                errors.append(f"Action {a['name']} 的 {end} 引用了未定义的 Object: {a[end]}")

    if errors:
        print("❌ 校验失败：")
        for e in errors:
            print(f"  - {e}")
        return 1
    print("✅ 本体校验通过")
    return 0

if __name__ == "__main__":
    sys.exit(validate(sys.argv[1] if len(sys.argv) > 1 else "ontology.yaml"))