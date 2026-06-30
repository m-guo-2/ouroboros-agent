# 路径规划 API 参考 (v5)

所有路径规划接口均为 v5 版本，GET 请求（参数过长时用 POST）。

## 驾车路线规划

**端点**: `GET https://restapi.amap.com/v5/direction/driving`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| origin | 是 | 起点坐标 | `116.45925,39.910031` |
| destination | 是 | 终点坐标 | `116.587922,40.081577` |
| strategy | 否 | 算路策略，默认 32 | `32` |
| waypoints | 否 | 途经点（最多 16 个），`;` 分隔 | `116.5,39.9;116.6,40.0` |
| avoidpolygons | 否 | 避让区域（最多 32 个），区域间 `\|` 分隔 | |
| avoidroad | 否 | 避让道路名 | `长安街` |
| province | 否 | 车牌省份（限行规避） | `京` |
| number | 否 | 车牌尾号 | `5` |
| cartype | 否 | 车辆类型：`0`=普通(默认), `1`=纯电动 | `0` |
| show_fields | 否 | 额外返回字段，逗号分隔 | `cost,navi,cities,polyline` |

### 驾车策略 strategy 值

| 值 | 策略 | 返回路线数 |
|----|------|-----------|
| 0 | 速度优先 | 1 |
| 1 | 费用优先（不走收费路段） | 1 |
| 2 | 常规最快 | 1 |
| 32 | 高德推荐（默认） | 多条 |
| 33 | 躲避拥堵 | 多条 |
| 34 | 高速优先 | 多条 |
| 35 | 不走高速 | 多条 |
| 36 | 少收费 | 多条 |
| 37 | 大路优先 | 多条 |
| 38 | 速度最快 | 多条 |
| 39 | 躲避拥堵+高速优先 | 多条 |
| 40 | 躲避拥堵+不走高速 | 多条 |
| 41 | 躲避拥堵+少收费 | 多条 |
| 42 | 少收费+不走高速 | 多条 |
| 43 | 躲避拥堵+少收费+不走高速 | 多条 |
| 44 | 躲避拥堵+大路优先 | 多条 |
| 45 | 躲避拥堵+速度最快 | 多条 |

### 返回结构

```json
{
  "status": "1",
  "route": {
    "origin": "116.45925,39.910031",
    "destination": "116.587922,40.081577",
    "taxi_cost": "80",
    "paths": [{
      "distance": "28126",
      "duration": "2400",
      "strategy": "速度最快",
      "steps": [{
        "instruction": "向北行驶100米右转",
        "road_name": "阜通东大街",
        "step_distance": "100",
        "orientation": "北",
        "duration": "20",
        "polyline": "116.481,39.989;116.481,39.990",
        "action": "右转",
        "assistant_action": "到达途经点"
      }]
    }]
  }
}
```

关键字段：
- `paths[].distance` — 总距离(米)
- `paths[].duration` — 总耗时(秒)
- `paths[].steps[]` — 导航步骤列表
- `route.taxi_cost` — 预估打车费(元)

---

## 步行路线规划

**端点**: `GET https://restapi.amap.com/v5/direction/walking`

参数：`key`, `origin`, `destination`, `show_fields`(可选)。返回结构同驾车。

---

## 公交路线规划

**端点**: `GET https://restapi.amap.com/v5/direction/transit/integrated`

### 请求参数

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| key | 是 | API Key | |
| origin | 是 | 起点坐标 | |
| destination | 是 | 终点坐标 | |
| city1 | 是 | 起点城市（adcode/citycode） | `010` |
| city2 | 否 | 终点城市（跨城时填） | `021` |
| strategy | 否 | `0`=最快(默认), `1`=最省, `2`=最少换乘, `3`=最少步行, `5`=不坐地铁, `7`=最舒适 | `0` |
| AlternativeRoute | 否 | 备选方案数，默认 1，最大 5 | `3` |
| show_fields | 否 | 额外字段：`cost,navi,polyline` | |

### 返回结构

```json
{
  "route": {
    "transits": [{
      "cost": { "duration": "3600", "transit_fee": "5" },
      "walking_distance": "800",
      "segments": [{
        "bus": {
          "buslines": [{
            "name": "地铁15号线",
            "departure_stop": { "name": "望京站" },
            "arrival_stop": { "name": "清华东路西口站" },
            "via_num": "5",
            "via_stops": []
          }]
        },
        "walking": { "distance": "200", "steps": [] }
      }]
    }]
  }
}
```

---

## 骑行路线规划

**端点**: `GET https://restapi.amap.com/v5/direction/bicycling`

参数同步行。返回结构同驾车。

---

## 电动车路线规划

**端点**: `GET https://restapi.amap.com/v5/direction/electrobike`

参数同步行。与骑行的区别：会考虑电动车限行区域。
