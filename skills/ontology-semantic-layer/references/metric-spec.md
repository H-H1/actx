# 指标语义层规格

## 必填字段

- `name`：英文标识，snake_case
- `display_name`：中文业务名
- `business_definition`：业务定义，必须含分子、分母、时间窗
- `formula`：可执行公式
- `numerator`：分子定义（description + sql）
- `denominator`：分母定义（description + sql）
- `time_window`：value + unit（day/week/month）
- `data_source`：table + date_field + member_field
- `owner`：负责人
- `update_frequency`：daily/weekly/monthly
- `status`：active/draft/deprecated
- `version`：语义化版本号

## 可选字段

- `filters`：过滤条件列表，每项含 description + sql
- `dimensions`：可下钻维度（如门店、渠道）
- `notes`：口径说明、历史变更

## 公式规范

- 分子分母分开写，不要合并成一行
- SQL 必须能在 `data_source` 指定的表上执行
- 时间窗在 SQL 里用参数占位（如 `{time_window}`），不要写死
- 过滤条件必须可翻译成 SQL WHERE 子句

## 不要做的事

- 不要只写指标名不写业务定义
- 不要写"复购率 = 复购人数/总人数"这种不可执行的公式
- 不要漏时间窗
- 不要漏剔除规则（虚假注册、已注销）
- 不要没有 owner