package model

type NotifyRecord struct {
	Id
	Txid string `gorm:"type:varchar(128);uniqueIndex;not null;comment:交易哈希"`
	AutoTimeAt
}

func (nr NotifyRecord) TableName() string {

	return "bep_notify"
}

// IsNeedNotifyByTxid 判断指定 txid 是否需要触发通知；
// 原实现为两次独立 SELECT（且 Find 会把整行加载），改成单次 UNION ALL 子查询只算行数，
// 一次 DB roundtrip 覆盖 bep_notify 和 bep_order 两表的存在性检测
func IsNeedNotifyByTxid(txid string) bool {
	var count int64
	Db.Raw(
		"SELECT COUNT(*) FROM ("+
			"SELECT 1 AS x FROM bep_notify WHERE txid = ? "+
			"UNION ALL "+
			"SELECT 1 AS x FROM bep_order WHERE ref_hash = ?"+
			") AS t",
		txid, txid,
	).Scan(&count)

	return count == 0
}
