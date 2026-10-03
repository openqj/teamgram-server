package msg

import "github.com/teamgram/proto/mtproto"

// Encode implements mtproto.TLObject for the internal mention-only read RPC.
func (m *TLMsgReadMentions) Encode(x *mtproto.EncodeBuf, layer int32) error {
	if uint32(m.GetConstructor()) != 0x1168a346 {
		return nil
	}
	x.UInt(0x1168a346)
	var flags uint32
	if m.GetTopMsgId() != nil {
		flags |= 1 << 0
	}
	x.UInt(flags)
	x.Long(m.GetUserId())
	x.Long(m.GetAuthKeyId())
	x.Int(m.GetPeerType())
	x.Long(m.GetPeerId())
	if m.GetTopMsgId() != nil {
		x.Int(m.GetTopMsgId().GetValue())
	}
	return nil
}

func (m *TLMsgReadMentions) CalcByteSize(layer int32) int {
	return 0
}

func (m *TLMsgReadMentions) Decode(dBuf *mtproto.DecodeBuf) error {
	if uint32(m.GetConstructor()) != 0x1168a346 {
		return dBuf.GetError()
	}
	flags := dBuf.UInt()
	m.UserId = dBuf.Long()
	m.AuthKeyId = dBuf.Long()
	m.PeerType = dBuf.Int()
	m.PeerId = dBuf.Long()
	if flags&(1<<0) != 0 {
		m.TopMsgId = mtproto.MakeFlagsInt32(dBuf.Int())
	}
	return dBuf.GetError()
}
