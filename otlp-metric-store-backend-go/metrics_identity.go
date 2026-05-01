package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

type metricMetadataIdentity struct {
	ResourceAttributes    map[string]string `json:"resource_attributes"`
	ScopeName             string            `json:"scope_name"`
	ScopeVersion          string            `json:"scope_version"`
	ScopeAttributes       map[string]string `json:"scope_attributes"`
	ScopeDroppedAttrCount uint32            `json:"scope_dropped_attr_count"`
	ServiceName           string            `json:"service_name"`
	MetricName            string            `json:"metric_name"`
	MetricUnit            string            `json:"metric_unit"`
	Attributes            map[string]string `json:"attributes"`
}

type sumMetadataIdentity struct {
	MetricMetadataIdentity metricMetadataIdentity `json:"metric_metadata_identity"`
	AggregationTemporality int32                  `json:"aggregation_temporality"`
	IsMonotonic            bool                   `json:"is_monotonic"`
}

type metadataReplacement struct {
	ResourceSchemaURL string `json:"resource_schema_url"`
	ScopeSchemaURL    string `json:"scope_schema_url"`
	MetricDescription string `json:"metric_description"`
}

func hash128(value any) [16]byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal hash payload: %v", err))
	}

	sum := sha256.Sum256(payload)

	var truncated [16]byte
	copy(truncated[:], sum[:16])
	return truncated
}

func metricMetadataIdentityPayload(row MetricMetadataRow) metricMetadataIdentity {
	return metricMetadataIdentity{
		ResourceAttributes:    row.ResourceAttributes,
		ScopeName:             row.ScopeName,
		ScopeVersion:          row.ScopeVersion,
		ScopeAttributes:       row.ScopeAttributes,
		ScopeDroppedAttrCount: row.ScopeDroppedAttrCount,
		ServiceName:           row.ServiceName,
		MetricName:            row.MetricName,
		MetricUnit:            row.MetricUnit,
		Attributes:            row.Attributes,
	}
}

func gaugeMetadataKey(row MetricMetadataRow) MetadataKey {
	return MetadataKey(hash128(metricMetadataIdentityPayload(row)))
}

func sumMetadataKey(row SumMetadataRow) MetadataKey {
	return MetadataKey(hash128(sumMetadataIdentity{
		MetricMetadataIdentity: metricMetadataIdentityPayload(row.MetricMetadataRow),
		AggregationTemporality: row.AggregationTemporality,
		IsMonotonic:            row.IsMonotonic,
	}))
}

func metadataReplacementRank(resourceSchemaURL string, scopeSchemaURL string, metricDescription string) ReplacementRank {
	return ReplacementRank(hash128(metadataReplacement{
		ResourceSchemaURL: resourceSchemaURL,
		ScopeSchemaURL:    scopeSchemaURL,
		MetricDescription: metricDescription,
	}))
}
