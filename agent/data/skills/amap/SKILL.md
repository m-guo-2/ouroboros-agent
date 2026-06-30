---
name: amap
description: "地图与位置服务。查路线、搜地点、查天气、查行政区划、地址坐标互转。当需要规划路线、搜索地点、查询天气或进行地理编码时使用。"
homepage: https://lbs.amap.com/api/webservice/summary
version: 2.0.0
metadata: {"openclaw":{"primaryEnv":"AMAP_API_KEY","category":"map","emoji":"🗺️"}}
---

# 地图与位置服务

通过 `run_script` 执行对应脚本完成操作。

## 查路线

直接传地址，脚本自动转坐标再规划路线：

```
run_script("amap", "route", "--from '北京南站' --to '故宫博物院'")
```

支持不同出行方式：

```
run_script("amap", "route", "--from '北京南站' --to '三里屯' --mode walking")
run_script("amap", "route", "--from '北京南站' --to '国贸' --mode transit --city 北京")
```

`--mode` 可选：driving（默认）、walking、bicycling、transit（公交需加 `--city`）。

## 搜地点

搜索地点或周边设施：

```
run_script("amap", "search_place", "--query '星巴克' --city '北京'")
```

搜附近（传中心坐标）：

```
run_script("amap", "search_place", "--query '加油站' --around '116.397,39.909' --radius 2000")
```

## 查天气

```
run_script("amap", "weather", "--city '北京'")
```

加 `--forecast` 返回未来 4 天预报。

## 地址坐标互转

地址转坐标：

```
run_script("amap", "geocode", "--address '北京市朝阳区阜通东大街6号'")
```

坐标转地址：

```
run_script("amap", "geocode", "--location '116.397499,39.908722'")
```

## 查行政区划

```
run_script("amap", "district", "--query '北京' --depth 2")
```

`--depth` 控制子级深度：0=不返回子级，1=省→市，2=省→市→区。

## 进阶用法

以上脚本覆盖常见场景。如需更细粒度的 API 参数控制，通过 `load_skill_reference` 查阅：

- `geocode_references.md` — 地理编码 / 逆地理编码
- `poi_references.md` — POI 搜索
- `route_references.md` — 路径规划
- `misc_references.md` — 天气、行政区域、IP 定位
