package siemens

import (
	"bytes"
	"encoding/binary"
	"net"
)

func GetS7Banner(logStruct *S7Log, connection net.Conn) (err error) {

	// -------- Attempt connection
	connPacketBytes, err := makeCOTPConnectionPacketBytes(uint16(0x102), uint16(0x100))
	//	connPacketBytes, err := makeCOTPConnectionPacket(uint16(0x200), uint16(0x100)).Marshal()
	if err != nil {
		return err
	}
	connResponseBytes, err := sendRequestReadResponse(connection, connPacketBytes)
	if err != nil {
		return err
	}
	_, err = unmarshalCOTPConnectionResponse(connResponseBytes)
	if err != nil {
		return err
	}

	// -------- Negotiate S7
	requestPacketBytes, err := makeRequestPacketBytes(S7_REQUEST, makeNegotiatePDUParamBytes(), nil)
	if err != nil {
		return err
	}
	_, err = sendRequestReadResponse(connection, requestPacketBytes)
	if err != nil {
		return err
	}

	logStruct.IsS7 = true

	// -------- Make Component Identification request
	readRequestBytes, err := makeReadRequestBytes(uint16(0x1c))
	if err != nil {
		return err
	}
	readResponse, err := sendRequestReadResponse(connection, readRequestBytes)
	if err != nil {
		return err
	}
	s7Packet, err := unmarshalReadResponse(readResponse)
	if err != nil {
		return err
	}
	parseComponentIdentificationResponse(logStruct, &s7Packet)

	return nil
}

func makeCOTPConnectionPacketBytes(dstTsap uint16, srcTsap uint16) ([]byte, error) {
	var cotpConnPacket COTPConnectionPacket
	cotpConnPacket.DestinationRef = uint16(0x00) // nmap uses 0x00
	cotpConnPacket.SourceRef = uint16(0x04)      // nmap uses 0x14
	cotpConnPacket.DestinationTSAP = dstTsap
	cotpConnPacket.SourceTSAP = srcTsap
	cotpConnPacket.TPDUSize = byte(0x0a) // nmap uses 0x0a

	cotpConnPacketBytes, err := cotpConnPacket.Marshal()
	if err != nil {
		return nil, err
	}

	var tpktPacket TPKTPacket
	tpktPacket.Data = cotpConnPacketBytes
	bytes, err := tpktPacket.Marshal()
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func makeRequestPacketBytes(pduType byte, parameters []byte, data []byte) ([]byte, error) {
	var s7Packet S7Packet
	s7Packet.PDUType = pduType
	s7Packet.RequestId = S7_REQUEST_ID
	s7Packet.Parameters = parameters
	s7Packet.Data = data
	s7PacketBytes, err := s7Packet.Marshal()
	if err != nil {
		return nil, err
	}

	var cotpDataPacket COTPDataPacket
	cotpDataPacket.Data = s7PacketBytes
	cotpDataPacketBytes, err := cotpDataPacket.Marshal()
	if err != nil {
		return nil, err
	}

	var tpktPacket TPKTPacket
	tpktPacket.Data = cotpDataPacketBytes
	bytes, err := tpktPacket.Marshal()
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

// Send a generic packet request and return the response
func sendRequestReadResponse(connection net.Conn, requestBytes []byte) ([]byte, error) {
	connection.Write(requestBytes)
	responseBytes := make([]byte, 1024)
	bytesRead, err := connection.Read(responseBytes)
	if err != nil {
		return nil, err
	}

	return responseBytes[0:bytesRead], nil
}

func unmarshalCOTPConnectionResponse(responseBytes []byte) (cotpConnPacket COTPConnectionPacket, err error) {
	var tpktPacket TPKTPacket
	if err := tpktPacket.Unmarshal(responseBytes); err != nil {
		return cotpConnPacket, err
	}

	if err := cotpConnPacket.Unmarshal(tpktPacket.Data); err != nil {
		return cotpConnPacket, err
	}

	return cotpConnPacket, nil
}

func makeNegotiatePDUParamBytes() (bytes []byte) {
	uint16BytesHolder := make([]byte, 2)
	bytes = make([]byte, 0, 8)        // fixed param length for negotiating PDU params
	bytes = append(bytes, byte(0xf0)) // negotiate PDU function code
	bytes = append(bytes, byte(0))    // ?
	binary.BigEndian.PutUint16(uint16BytesHolder, 0x01)
	bytes = append(bytes, uint16BytesHolder...) // min # of parallel jobs
	binary.BigEndian.PutUint16(uint16BytesHolder, 0x01)
	bytes = append(bytes, uint16BytesHolder...) // max # of parallel jobs

	return bytes
}

func makeReadRequestParamBytes(data []byte) (bytes []byte) {
	bytes = make([]byte, 0, 16)

	bytes = append(bytes, byte(0x00)) // magic parameter
	bytes = append(bytes, byte(0x01)) // magic parameter
	bytes = append(bytes, byte(0x12)) // magic parameter
	bytes = append(bytes, byte(0x04)) // param length
	bytes = append(bytes, byte(0x11)) // ?
	bytes = append(bytes, byte((S7_SZL_REQUEST*0x10)+S7_SZL_FUNCTIONS))
	bytes = append(bytes, byte(S7_SZL_READ))
	bytes = append(bytes, byte(0))

	return bytes
}

func makeReadRequestDataBytes(szlId uint16) []byte {
	bytes := make([]byte, 0, 4)
	bytes = append(bytes, byte(0xff))
	bytes = append(bytes, byte(0x09))
	uint16BytesHolder := make([]byte, 2)
	binary.BigEndian.PutUint16(uint16BytesHolder, uint16(4)) // size of subsequent data
	bytes = append(bytes, uint16BytesHolder...)
	binary.BigEndian.PutUint16(uint16BytesHolder, szlId)
	bytes = append(bytes, uint16BytesHolder...) // szl id
	binary.BigEndian.PutUint16(uint16BytesHolder, 1)
	bytes = append(bytes, uint16BytesHolder...) // szl index

	return bytes
}

func makeReadRequestBytes(szlId uint16) ([]byte, error) {
	readRequestParamBytes := makeReadRequestParamBytes(makeReadRequestDataBytes(szlId))
	readRequestBytes, err := makeRequestPacketBytes(S7_REQUEST_USER_DATA, readRequestParamBytes, makeReadRequestDataBytes(szlId))
	if err != nil {
		return nil, err
	}

	return readRequestBytes, nil
}

func unmarshalReadResponse(bytes []byte) (S7Packet, error) {
	var tpktPacket TPKTPacket
	var cotpDataPacket COTPDataPacket
	var s7Packet S7Packet
	if err := tpktPacket.Unmarshal(bytes); err != nil {
		return s7Packet, err
	}

	if err := cotpDataPacket.Unmarshal(tpktPacket.Data); err != nil {
		return s7Packet, err
	}

	if err := s7Packet.Unmarshal(cotpDataPacket.Data); err != nil {
		return s7Packet, err
	}

	return s7Packet, nil
}

func parseComponentIdentificationResponse(logStruct *S7Log, s7Packet *S7Packet) {
	fields := bytes.FieldsFunc(s7Packet.Data[S7_DATA_BYTE_OFFSET:], func(c rune) bool {
		return int(c) == 0
	})

	for i := len(fields) - 1; i >= 0; i-- {
		switch i {
		case 0:
			logStruct.System = string(fields[i][1:]) // exclude index byte
		case 1:
			logStruct.Module = string(fields[i][1:])
		case 2:
			logStruct.PlantId = string(fields[i][1:])
		case 3:
			logStruct.Copyright = string(fields[i][1:])
		case 4:
			logStruct.SerialNumber = string(fields[i][1:])
		case 5:
			logStruct.ReservedForOS = string(fields[i][1:])
		case 6:
			logStruct.ModuleType = string(fields[i][1:])
		case 7:
			logStruct.MemorySerialNumber = string(fields[i][1:])
		case 8:
			logStruct.CpuProfile = string(fields[i][1:])
		case 9:
			logStruct.OEMId = string(fields[i][1:])
		case 10:
			logStruct.Location = string(fields[i][1:])
		}
	}
}
