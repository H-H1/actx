# 本体规格

## Object

- 必须有 `name`（英文）、`display_name`（中文）、`primary_key`
- `primary_key` 必须是唯一的、稳定的标识符
- `properties` 每项必须有 `name` 和 `type`
- `type` 取值：string / decimal / datetime / enum / boolean
- enum 类型必须有 `values`
- 敏感字段（手机号、身份证）必须标 `masked: true`

## Link

- 必须有 `name`、`display_name`、`from`、`to`、`cardinality`
- `cardinality` 取值：one_to_one / one_to_many / many_to_one / many_to_many
- `from` 和 `to` 必须引用已定义的 Object

## Action

- 必须有 `name`、`display_name`、`subject`、`object`
- `subject` 和 `object` 必须引用已定义的 Object
- 建议定义 `input` 和 `output`，便于 W8 执行型 Agent 使用

## 命名规范

- Object/Link/Action 的 `name` 用 snake_case 英文
- `display_name` 用中文，面向业务人员
- 不要用数据库表名当 Object name（如 `t_member` 不行，用 `Member`）

## 不要做的事

- 不要把属性当 Object（"会员等级"是 Member 的属性，不是 Object）
- 不要用非唯一字段当主键（如 name）
- 不要定义没有 cardinality 的 Link
- 不要引用未定义的 Object