---
name: ontology-semantic-layer
description: 为 FDE 项目生成业务本体模型（Object/Link/Action）和指标语义层（指标口径），输出可映射到标准产品扩展接口的 YAML/JSON 配置。当需要把客户业务名词、动词、关系建成机器可读模型，或把模糊指标变成可查询定义时使用。
---

# W5 本体与语义层生成

## 何时使用

- 需要把访谈里的业务名词/动词变成 Object/Link/Action
- 需要把"复购率"这类模糊指标变成可查询、可追溯的语义层定义
- 需要产出可映射到标准产品本体扩展接口/指标语义扩展接口的配置文件

## 输入

1. 访谈纪要（W2 产出）
2. 指标口径公式（W3 产出）
3. 业务实体清单、动作清单、关系清单
4. 数据来源信息（表、字段）

## 输出

- `ontology.yaml`：Object/Link/Action 定义
- `metrics.yaml`：指标语义层定义

## 步骤

### 第一部分：本体模型

1. 从访谈纪要提取名词和动词
2. 判断哪些名词是 Object（有独立身份、能被单独查询）
3. 定 Object 的主键和属性类型
4. 把动词建成 Action（定义 subject/object）
5. 把关系建成 Link（定义 from/to/cardinality）
6. 按 `assets/ontology-template.yaml` 写 YAML
7. 运行 `scripts/validate_ontology.py` 校验

### 第二部分：指标语义层

1. 把业务定义写清楚（分子、分母、时间窗）
2. 把公式写成可执行 SQL 或表达式
3. 定数据来源（表、日期字段、会员字段）
4. 定过滤条件、负责人、更新频率、版本
5. 按 `assets/metric-template.yaml` 写 YAML
6. 运行 `scripts/validate_metric.py` 校验

## 参考

- 本体规格：`references/ontology-spec.md`
- 指标规格：`references/metric-spec.md`
- 校验规则：`references/validation-rules.md`
- 示例：`assets/example-ontology.yaml`、`assets/example-metric.yaml`

## 校验

两个脚本都必须通过，且无 warning。

## 常见错误

见 `references/validation-rules.md` 的"常见错误"章节。