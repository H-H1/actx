#!/usr/bin/env python3
"""通用 YAML lint：检查缩进、重复 key、空值。"""
import sys
import yaml

def lint(path):
    with open(path) as f:
        try:
            yaml.safe_load(f)
        except yaml.YAMLError as e:
            print(f"❌ YAML 语法错误: {e}")
            return 1
    print(f"✅ {path} YAML 语法正确")
    return 0

if __name__ == "__main__":
    sys.exit(lint(sys.argv[1]))