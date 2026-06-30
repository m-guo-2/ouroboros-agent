# 天气查询 / 行政区域查询 / IP 定位 API 参考

## 天气查询

**端点**: `GET https://restapi.amap.com/v3/weather/weatherInfo`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| city | 是 | 城市 adcode | `110000` |
| extensions | 否 | `base`(实况，默认) / `all`(预报) | `all` |

### 返回字段 (extensions=base 实况天气)

```json
{
  "status": "1",
  "lives": [{
    "province": "北京",
    "city": "北京市",
    "adcode": "110000",
    "weather": "晴",
    "temperature": "25",
    "winddirection": "南",
    "windpower": "≤3",
    "humidity": "45",
    "reporttime": "2026-03-12 15:00:00"
  }]
}
```

### 返回字段 (extensions=all 天气预报)

```json
{
  "status": "1",
  "forecasts": [{
    "city": "北京市",
    "adcode": "110000",
    "reporttime": "2026-03-12 11:00:00",
    "casts": [{
      "date": "2026-03-12",
      "week": "4",
      "dayweather": "晴",
      "nightweather": "多云",
      "daytemp": "26",
      "nighttemp": "12",
      "daywind": "南",
      "nightwind": "南",
      "daypower": "≤3",
      "nightpower": "≤3"
    }]
  }]
}
```

`casts` 数组包含当天和未来 3 天共 4 条预报数据。

---

## 行政区域查询

**端点**: `GET https://restapi.amap.com/v3/config/district`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| keywords | 否 | 行政区名称、citycode 或 adcode，不填返回全国 | `北京` |
| subdistrict | 否 | 返回子级层数：`0`=不返回, `1`=一级(默认), `2`=二级, `3`=三级 | `1` |
| filter | 否 | 限定行政区搜索范围（adcode） | `110000` |
| extensions | 否 | `base`(默认，不返回边界) / `all`(返回边界坐标) | `base` |

### 返回字段

```json
{
  "status": "1",
  "districts": [{
    "citycode": "010",
    "adcode": "110000",
    "name": "北京市",
    "center": "116.407526,39.904030",
    "level": "province",
    "districts": [{
      "citycode": "010",
      "adcode": "110101",
      "name": "东城区",
      "center": "116.418757,39.917544",
      "level": "district",
      "districts": []
    }]
  }]
}
```

`level` 取值：`country` / `province` / `city` / `district` / `street`

---

## IP 定位

**端点**: `GET https://restapi.amap.com/v5/ip/location`（高级服务，需确认 Key 有权限）

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| ip | 是 | IP 地址 | `114.247.50.2` |
| type | 否 | IP 类型：`4`(IPv4) / `6`(IPv6) | `4` |

### 返回字段

```json
{
  "status": "1",
  "country": "中国",
  "province": "北京市",
  "city": "北京市",
  "district": "海淀区",
  "isp": "中国联通",
  "location": "116.310003,39.991957",
  "ip": "114.247.50.2"
}
```

### 备用 v3 接口（无需额外权限，精度仅到城市）

```
GET https://restapi.amap.com/v3/ip?key={key}&ip={ip}
```
