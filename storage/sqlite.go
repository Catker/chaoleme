package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// MetricType 指标类型
type MetricType string

const (
	MetricTypeCPUSteal  MetricType = "cpu_steal"
	MetricTypeCPUIoWait MetricType = "cpu_iowait"
	MetricTypeCPUBench  MetricType = "cpu_bench"
	MetricTypeIOLatency MetricType = "io_latency"
	MetricTypeDiskStats MetricType = "disk_stats" // 磁盘统计（IOPS/吞吐量）
	MetricTypeRandomIO  MetricType = "random_io"  // 随机 IO 延迟
	MetricTypeMemory    MetricType = "memory"
	MetricTypeCPULoad   MetricType = "cpu_load"
)

// Metric 指标数据
type Metric struct {
	ID        int64
	Timestamp time.Time
	Type      MetricType
	Value     float64
	Extra     map[string]interface{}
}

// Storage 数据存储
type Storage struct {
	db *sql.DB
}

// New 创建存储实例
func New(dbPath string) (*Storage, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	s := &Storage{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// init 初始化数据库表
func (s *Storage) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		metric_type TEXT NOT NULL,
		value REAL NOT NULL,
		extra TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_metrics_time ON metrics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_metrics_type ON metrics(metric_type, timestamp);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("初始化数据库表失败: %w", err)
	}

	return nil
}

// Close 关闭数据库连接
func (s *Storage) Close() error {
	return s.db.Close()
}

// Save 保存指标数据
func (s *Storage) Save(m *Metric) error {
	var extraJSON []byte
	var err error

	if m.Extra != nil {
		extraJSON, err = json.Marshal(m.Extra)
		if err != nil {
			return fmt.Errorf("序列化 extra 失败: %w", err)
		}
	}

	_, err = s.db.Exec(
		"INSERT INTO metrics (timestamp, metric_type, value, extra) VALUES (?, ?, ?, ?)",
		m.Timestamp.Unix(),
		string(m.Type),
		m.Value,
		string(extraJSON),
	)

	if err != nil {
		return fmt.Errorf("保存指标失败: %w", err)
	}

	return nil
}

// Query 查询指定时间范围和类型的指标
func (s *Storage) Query(metricType MetricType, start, end time.Time) ([]*Metric, error) {
	rows, err := s.db.Query(
		"SELECT id, timestamp, metric_type, value, extra FROM metrics WHERE metric_type = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		string(metricType),
		start.Unix(),
		end.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("查询指标失败: %w", err)
	}
	defer rows.Close()

	var metrics []*Metric
	for rows.Next() {
		m := &Metric{}
		var ts int64
		var typeStr string
		var extraStr sql.NullString

		if err := rows.Scan(&m.ID, &ts, &typeStr, &m.Value, &extraStr); err != nil {
			return nil, fmt.Errorf("扫描行失败: %w", err)
		}

		m.Timestamp = time.Unix(ts, 0)
		m.Type = MetricType(typeStr)

		if extraStr.Valid && extraStr.String != "" {
			if err := json.Unmarshal([]byte(extraStr.String), &m.Extra); err != nil {
				// 忽略解析错误
				m.Extra = nil
			}
		}

		metrics = append(metrics, m)
	}

	return metrics, nil
}

// Cleanup 清理过期数据
func (s *Storage) Cleanup(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	result, err := s.db.Exec("DELETE FROM metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期数据失败: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return deleted, nil
}

// GetLatestMetric 获取最新的指标
func (s *Storage) GetLatestMetric(metricType MetricType) (*Metric, error) {
	row := s.db.QueryRow(
		"SELECT id, timestamp, metric_type, value, extra FROM metrics WHERE metric_type = ? ORDER BY timestamp DESC LIMIT 1",
		string(metricType),
	)

	m := &Metric{}
	var ts int64
	var typeStr string
	var extraStr sql.NullString

	if err := row.Scan(&m.ID, &ts, &typeStr, &m.Value, &extraStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("获取最新指标失败: %w", err)
	}

	m.Timestamp = time.Unix(ts, 0)
	m.Type = MetricType(typeStr)

	if extraStr.Valid && extraStr.String != "" {
		json.Unmarshal([]byte(extraStr.String), &m.Extra)
	}

	return m, nil
}
