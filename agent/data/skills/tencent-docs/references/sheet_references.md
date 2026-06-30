# 在线表格（Sheet）API 参考

本文件包含腾讯文档在线表格（Excel 格式）相关 API 的完整说明、详细调用示例、参数说明和返回值说明。

---

## 通用说明

### Sheet API 概述

Sheet API 专门用于操作腾讯文档中的在线表格（Excel格式），提供表格信息查询、范围数据读取、批量更新和 JS 脚本操作等功能。

### 响应结构

所有 API 返回都包含：
- `error`: 错误信息（成功时为空）
- `trace_id`: 调用链追踪 ID

### 表格范围表示法

Sheet API 使用 A1 表示法来指定表格范围：
- `A1`: 单个单元格
- `A1:B10`: 矩形区域

---

## API 调用示例

## 1. sheet.get_info

### 功能说明
查询工作表的基本信息，包括所有子表的ID、标题、大小和已使用的行列数。

### 参数

```json
{
  "file_id": "sheet_1234567890",
  "concise": false
}
```

### 参数说明
- `file_id` (string, 必填): 在线表格唯一标识符
- `concise` (boolean, 可选): 是否返回简洁信息，true 不包含子表使用行列数，false 返回完整信息

### 返回值
```json
{
  "sheet_info": {
    "file_id": "sheet_1234567890",
    "title": "销售数据表",
    "sheets": [
      {
        "sheet_id": "sht1234567890",
        "title": "Sheet1",
        "row_count": 100,
        "column_count": 10,
        "used_row_count": 50,
        "used_column_count": 5
      }
    ]
  },
  "error": "",
  "trace_id": "trace_1234567890"
}
```

## 2. sheet.get_range

### 功能说明
获取指定范围内的在线表格数据，支持 A1 表示法指定查询范围。

### 参数

```json
{
  "file_id": "sheet_1234567890",
  "sheet_id": "sht1234567890",
  "range": "A1:C10"
}
```

### 参数说明
- `file_id` (string, 必填): 在线表格唯一标识符
- `sheet_id` (string, 必填): 工作表 ID
- `range` (string, 必填): 查询范围，使用 A1 表示法（如 `A1:D10`）

### 返回值
```json
{
  "range_data": {
    "range": "A1:C10",
    "values": [
      ["姓名", "年龄", "部门"],
      ["张三", "25", "技术部"],
      ["李四", "30", "产品部"]
    ]
  },
  "error": "",
  "trace_id": "trace_1234567890"
}
```

## 3. sheet.batch_update

### 功能说明
批量执行对在线表格的更新操作，支持添加工作表、更新单元格内容、删除行列、删除工作表。单次请求最多 5 个操作。

### 参数

```json
{
  "file_id": "sheet_1234567890",
  "requests": [
    {
      "add_sheet": {
        "title": "新工作表",
        "row_count": 200,
        "column_count": 20
      }
    },
    {
      "update_range": {
        "sheet_id": "sht1234567890",
        "grid_data": {
          "start_row": 0,
          "start_column": 0,
          "rows": [
            {"values": [{"cell_value": {"text": "标题1"}}, {"cell_value": {"text": "标题2"}}]},
            {"values": [{"cell_value": {"number": 100}}, {"cell_value": {"number": 200}}]}
          ]
        }
      }
    }
  ]
}
```

### 参数说明
- `file_id` (string, 必填): 在线表格唯一标识符
- `requests` (array, 必填): 批量操作请求列表，单次最多 5 个操作

### 支持的操作类型

#### 添加工作表 (add_sheet)
```json
{
  "add_sheet": {
    "title": "工作表标题",
    "row_count": 200,
    "column_count": 20
  }
}
```
- `title` (string): 工作表名称，限31个字符
- `row_count` (integer): 初始行数，默认200
- `column_count` (integer): 初始列数，默认20
- row_count * column_count 不能超过 10000

#### 更新单元格范围 (update_range)
```json
{
  "update_range": {
    "sheet_id": "sht1234567890",
    "grid_data": {
      "start_row": 0,
      "start_column": 0,
      "rows": [
        {
          "values": [
            {
              "cell_value": {"text": "文本值"},
              "cell_format": {
                "text_format": {"bold": true, "font_size": 12}
              }
            }
          ]
        }
      ]
    }
  }
}
```

**cell_value 支持的值类型**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `text` | string | 文本值 |
| `number` | number | 数字值 |
| `link` | object | 超链接，含 `text` 和 `url` |
| `time` | object | 时间，含 `year`/`month`/`day`/`hour`/`minute`/`second` |
| `select` | object | 选择项，含 `multiple`、`options`、`value` |
| `location` | object | 地理位置，含 `name`/`latitude`/`longitude` |

**cell_format.text_format 支持的格式**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `bold` | boolean | 粗体 |
| `italic` | boolean | 斜体 |
| `underline` | boolean | 下划线 |
| `strikethrough` | boolean | 删除线 |
| `font` | string | 字体名称，默认宋体 |
| `font_size` | integer | 字号，范围 (0,72] |
| `color` | object | 字体颜色，含 `red`/`green`/`blue`/`alpha`（0-255） |

#### 删除行列 (delete_dimension)
```json
{
  "delete_dimension": {
    "sheet_id": "sht1234567890",
    "dimension": "ROW",
    "start_index": 5,
    "end_index": 10
  }
}
```
- `dimension`: `ROW`（行）或 `COLUMN`（列）
- `start_index`/`end_index`: 从1开始，左闭右开 `[start, end)`

#### 删除工作表 (delete_sheet)
```json
{
  "delete_sheet": {
    "sheet_id": "sht1234567890"
  }
}
```

### 返回值
```json
{
  "replies": [
    {
      "add_sheet": {
        "properties": {
          "sheet_id": "sht1234567890",
          "title": "新工作表",
          "index": 1
        }
      }
    }
  ],
  "error": "",
  "trace_id": "trace_1234567890"
}
```

## 4. sheet.operation_sheet

### 功能说明
通过 JS 脚本精细化操作在线表格，支持读写单元格值、设置公式、设置背景色/字体色、插入删除行列、设置行高列宽、查询子表信息等。

### 参数

```json
{
  "file_id": "sheet_1234567890",
  "sheet_id": "sht1234567890",
  "js_script": "const value = Spreadsheet.getCellValue(0, 0); Spreadsheet.setCellValue(0, 1, value * 2);"
}
```

### 参数说明
- `file_id` (string, 必填): 在线表格唯一标识符
- `js_script` (string, 必填): 在沙箱中执行的 JS 脚本，通过 `Spreadsheet` 对象访问表格数据。行列索引从 0 开始
- `sheet_id` (string, 可选): 要操作的子表 ID

### 返回值
```json
{
  "result": "脚本执行结果",
  "error": "",
  "trace_id": "trace_1234567890"
}
```

---

## 注意事项

### 范围限制
- `sheet.get_range` 单次查询：行数 ≤ 1000，列数 ≤ 200，单元格总数 ≤ 10000
- `sheet.batch_update` 单次请求操作数量 ≤ 5
- `add_sheet` 创建的工作表单元格总数（row_count × column_count）≤ 10000

### 数据格式
- 单元格数据使用类型化结构（`cell_value` 包含 text/number/link/time 等）
- 空单元格使用空字符串表示

### 性能建议
- 大数据量更新建议使用 `sheet.batch_update` 批量操作
- 查询大范围数据时注意范围限制
- 复杂操作可使用 `sheet.operation_sheet` 的 JS 脚本
