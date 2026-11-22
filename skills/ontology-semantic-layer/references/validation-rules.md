# 校验规则

## 本体校验（validate_ontology.py 检查项）

1. 每个 Object 有 name、display_name、primary_key
2. 每个 property 有 name、type
3. enum 类型有 values
4. 每个 Link 有 from、to、cardinality
5. Link 的 from/to 引用的 Object 已定义
6. 每个 Action 有 subject、object
7. Action 的 subject/object 引用的 Object 已定义
8. 敏感字段标了 masked: true

## 指标校验（validate_metric.py 检查项）

1. 有 business_definition，且含"分子/分母/时间窗"关键词
2. 有 formula
3. numerator 和 denominator 都有 description 和 sql
4. 有 time_window（value + unit）
5. 有 data_source（table + date_field + member_field）
6. 有 owner
7. 有 version
8. filters 每项有 description 和 sql

## 常见错误

| 错误 | 表现 | 修正 |
|---|---|---|
| 属性当 Object | "会员等级"建成 Object | 改成 Member 的属性 |
| 主键不唯一 | 用 name 当主键 | 用 master_id |
| Link 缺基数 | 只写 from/to | 加 cardinality |
| 引用未定义 Object | Link 用了 Coupon 但没定义 | 补 Coupon 定义 |
| 指标定义模糊 | 只写"复购率" | 写清分子分母时间窗 |
| 公式不可执行 | 写"复购率=复购人数/总人数" | 写成 SQL |
| 缺时间窗 | 没写 90 天还是 30 天 | 加 time_window |
| 缺 owner | 没人负责 | 加 owner |