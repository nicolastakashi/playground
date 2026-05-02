package main

import (
	"time"

	"github.com/google/uuid"
)

type MetadataKey = uuid.UUID

type ReplacementRank [16]byte

type MetricMetadataRow struct {
	MetadataKey           MetadataKey
	ReplacementRank       ReplacementRank
	ResourceAttributes    map[string]string
	ResourceSchemaUrl     string
	ScopeName             string
	ScopeVersion          string
	ScopeAttributes       map[string]string
	ScopeDroppedAttrCount uint32
	ScopeSchemaUrl        string
	ServiceName           string
	MetricName            string
	MetricDescription     string
	MetricUnit            string
	Attributes            map[string]string
}

type SumMetadataRow struct {
	MetricMetadataRow
	AggregationTemporality int32
	IsMonotonic            bool
}

type GaugeDataPointRow struct {
	MetadataKey   MetadataKey
	StartTimeUnix time.Time
	TimeUnix      time.Time
	Value         float64
	Flags         uint32
}

type SumDataPointRow struct {
	GaugeDataPointRow
}

type NormalizedGaugeRows struct {
	Metadata   []MetricMetadataRow
	DataPoints []GaugeDataPointRow
}

type NormalizedSumRows struct {
	Metadata   []SumMetadataRow
	DataPoints []SumDataPointRow
}
