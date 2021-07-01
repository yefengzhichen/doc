## 基本概念

Elasticsearch 是什么？

		分布式、RESTful 风格的搜索和数据分析引擎



- Index索引：具有相似特性的文档集合，类比数据库表
- Document文档：相当于SQL里的一行记录
- Field字段：相当于SQL里的一个字段
- Shard分片：一个索引可切分为多个shard，分布在不同机器节点。每个shard都是一个lucene index
- Replica shard副本分片： shard故障备用，提升搜索吞吐量和性能
- Mapping映射：定义索引中字段的类型和索引设置
- Master节点：集群的主节点，它将负责管理集群范围的变更，例如创建或删除索引、从集群添加节点或删除节点

![img](https://upload-images.jianshu.io/upload_images/2400535-ed766630e41178c2.png?imageMogr2/auto-orient/strip|imageView2/2/w/617/format/webp)



## 基本操作

kibana操作：http://10.248.33.26:30601/app/kibana#/dev_tools/console?_g=()

#### 索引操作

增加索引：

```
PUT test_index
{
    "mappings" : {
      "dynamic": false,
      "properties" : {
        "create_time" : {
          "type" : "date"
        },
        "id" : {
          "type" : "keyword"
        },
        "qid" : {
          "type" : "long"
        },
        "subcategory_id" : {
          "type" : "keyword"
        }
      }
    },
    "settings": {
      "index": {
        "refresh_interval" : "30s"
      }
    }
}
```

查询索引：

```
GET test_index/_mapping
GET test_index
```

修改索引

```
PUT test_index/_mapping
{
  "properties": {
    "update_time": {
      "type": "date"
    }
  }
}

修改：修改原来字段为另一个类型，无效
PUT test_index/_mapping
{
  "properties": {
    "create_time": {
      "type": "keyword"
    }
  }
}

```

删除索引：

```
DELETE test_index
```



#### 文档操作
