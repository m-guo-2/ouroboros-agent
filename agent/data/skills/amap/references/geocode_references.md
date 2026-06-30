# 地理编码 / 逆地理编码 API 参考

## 地理编码（地址→坐标）

**端点**: `GET https://restapi.amap.com/v3/geocode/geo`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| address | 是 | 结构化地址（省+市+区+街道+门牌号） | `北京市朝阳区阜通东大街6号` |
| city | 否 | 指定城市（中文/全拼/citycode/adcode），不填则全国搜索 | `北京` / `010` / `110000` |
| output | 否 | 返回格式 `JSON`(默认) / `XML` | `JSON` |

### 返回字段

```json
{
  "status": "1",
  "count": "1",
  "geocodes": [{
    "formatted_address": "北京市朝阳区阜通东大街6号",
    "country": "中国",
    "province": "北京市",
    "city": "北京市",
    "citycode": "010",
    "district": "朝阳区",
    "adcode": "110105",
    "street": "阜通东大街",
    "number": "6号",
    "location": "116.480881,39.989410",
    "level": "门牌号"
  }]
}
```

关键字段：
- `location` — 经度,纬度（核心输出）
- `level` — 匹配级别：国家/省/市/区县/乡镇/道路/兴趣点/门牌号等

---

## 逆地理编码（坐标→地址）

**端点**: `GET https://restapi.amap.com/v3/geocode/regeo`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| location | 是 | 经度,纬度 | `116.480881,39.989410` |
| extensions | 否 | `base`(默认): 基本地址; `all`: 含附近 POI/道路/交叉口 | `all` |
| radius | 否 | POI 搜索半径(米)，0~3000，默认 1000 | `1000` |
| poitype | 否 | 限定 POI 类型（TYPECODE），多个用 `\|` 分隔，需 extensions=all | `060100\|060102` |
| roadlevel | 否 | `0`=所有道路, `1`=仅主干道，需 extensions=all | `1` |
| homeorcorp | 否 | POI 排序：`0`=默认, `1`=居家优先, `2`=公司优先，需 extensions=all | `0` |

### 返回字段 (extensions=base)

```json
{
  "status": "1",
  "regeocode": {
    "formatted_address": "北京市朝阳区望京街道方恒国际中心B座方恒国际中心",
    "addressComponent": {
      "country": "中国",
      "province": "北京市",
      "city": [],
      "citycode": "010",
      "district": "朝阳区",
      "adcode": "110105",
      "township": "望京街道",
      "towncode": "110105026000",
      "neighborhood": { "name": "方恒国际中心", "type": "商务住宅;楼宇;商务写字楼" },
      "building": { "name": "方恒国际中心B座", "type": "商务住宅;楼宇;商务写字楼" },
      "streetNumber": {
        "street": "阜通东大街",
        "number": "6号",
        "location": "116.481,39.9893",
        "direction": "东北",
        "distance": "67.2"
      }
    }
  }
}
```

extensions=all 额外返回：
- `roads[]` — 附近道路（id, name, distance, direction, location）
- `roadinters[]` — 道路交叉口（distance, direction, first_name, second_name）
- `pois[]` — 附近 POI（id, name, type, tel, distance, direction, address, location）
- `aois[]` — 附近 AOI（id, name, adcode, location, area, distance, type）

注意：直辖市（北京/上海/天津/重庆）的 `city` 字段返回空数组。
