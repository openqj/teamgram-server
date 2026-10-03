// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"encoding/hex"
	"net"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

func (d *Dao) CheckApiIdAndHash(apiId int32, apiHash string) error {
	// Validate syntax only; there is no trusted registry here to authenticate
	// the api_id/api_hash pair.
	if apiId <= 0 || len(apiHash) != 32 {
		return mtproto.ErrApiIdInvalid
	}
	if _, err := hex.DecodeString(apiHash); err != nil {
		return mtproto.ErrApiIdInvalid
	}

	return nil
}

func (d *Dao) GetCountryAndRegionByIp(ip string) (string, string) {
	if d.MMDB == nil {
		return "UNKNOWN", ""
	} else {
		r, err := d.MMDB.City(net.ParseIP(ip))
		if err != nil {
			logx.Errorf("getCountryAndRegionByIp - error: %v", err)
			return "UNKNOWN", ""
		}

		return r.City.Names["en"] + ", " + r.Country.Names["en"], r.Country.IsoCode
	}
}
