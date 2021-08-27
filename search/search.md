## 搜索逻辑



### 按精准意图聚合

先cond_md5分组，再按精准意图id分组进，最后按question_id分组统计，并且每个分组都会按更新时间排序：

![image-20210818173959621](/Users/yefeng/Library/Application Support/typora-user-images/image-20210818173959621.png)

1、有搜索问题关键词

​	得到一批问题排序的qids，根据请求的skip和limit，根据上面es中返回的分组排序结果，获取当前页数据，然后组装详细信息返回

2、非搜索问题关键词

​	根据请求的skip和limit，根据上面es中返回的分组排序结果，获取当前页数据，然后组装详细信息返回	



获取所有精准意图分组排序结果：

```json
{
	"aggregations": {
		"distinct_intent": {
			"cardinality": {
				"field": "conds.shop_intent_id",
				"precision_threshold": 40000
			}
		},
		"intent_agg": {
			"aggregations": {
				"conds_md5_agg": {
					"aggregations": {
						"max_update_time": {
							"max": {
								"field": "update_time"
							}
						}
					},
					"terms": {
						"field": "conds_md5", //按条件md5聚合
						"order": [{
							"max_update_time": "desc"
						}],
						"size": 1
					}
				},
				"max_update_time": {
					"max": {
						"field": "update_time"
					}
				}
			},
			"terms": {
				"field": "conds.shop_intent_id", //按精准意图id聚合
				"order": [{
					"max_update_time": "desc"
				}],
				"size": 100000
			}
		}
	},
	"terms": {
		"field": "question_id", //按问题id聚合
		"size": 25
	}
}
```



获取当前页的精准意图答案详情：

```
{
	"bool": {
		"minimum_should_match": "1",
		"should": {
			"bool": {
				"filter": [{
					"term": {
						"_index": "shop_condition_answer"
					}
				}, {
					"term": {
						"shop_id": "5fd6d3e46b85f2000efad55c"
					}
				}, {
					"terms": {
						"qid_md5": ["5639bf1b89bc4603d5c61395.23dcf056ee079ba37ac52f7314d81639", "5639bf1b89bc4603d5c612d6.08b5eb81144572b53431b1ec5587b02c", "5875ea9548d76c3af3ecf143.c1c34c24babd82890646c34f33251f9c", "5d6e2f8dd053390012961cea.b1cabb646613fb1bb2e7ade15028adf1", "5bf7e7409bac08600c653030.ab971199bf5a34371ac6a199445e4c87"]
					}
				}]
			}
		}
	}
}
```



