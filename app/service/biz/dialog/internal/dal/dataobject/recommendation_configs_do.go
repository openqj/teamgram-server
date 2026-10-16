/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dataobject

type RecommendationConfigsDO struct {
	Id          int32  `db:"id" json:"id"`
	ConfigKey   string `db:"config_key" json:"config_key"`
	ConfigValue string `db:"config_value" json:"config_value"`
	Enabled     bool   `db:"enabled" json:"enabled"`
	Description string `db:"description" json:"description"`
	CreatedAt   int64  `db:"created_at" json:"created_at"`
	UpdatedAt   int64  `db:"updated_at" json:"updated_at"`
}
