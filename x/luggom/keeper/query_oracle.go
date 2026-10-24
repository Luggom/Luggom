package keeper

import (
	"context"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"luggom/x/luggom/types"
)

// OracleFixing devolve a fixing de um dia (YYYY-MM-DD), resolvida pela tabela
// embutida primeiro e pelo store depois — a mesma via que a mineração usa.
//
// Serve duas coisas: auditoria (o registo on-chain deixa de ser opaco) e o
// feeder, que precisa da fixing anterior para limitar um salto e voltar ao
// valor verdadeiro aos poucos.
func (q queryServer) OracleFixing(ctx context.Context, req *types.QueryOracleFixingRequest) (*types.QueryOracleFixingResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	day := strings.TrimSpace(req.Day)
	if _, err := time.Parse(oracleDayLayout, day); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "day must be YYYY-MM-DD: %v", err)
	}

	snap, found, err := q.k.oracleFixingForDay(ctx, day)
	if err != nil {
		// O erro vem da tabela compilada (artefacto de build), não de estado:
		// devolvê-lo não expõe nada e evita um "Internal error" opaco.
		return nil, status.Errorf(codes.Internal, "oracle fixing: %v", err)
	}
	if !found {
		zero := sdkmath.LegacyZeroDec()
		return &types.QueryOracleFixingResponse{
			Day: day, Found: false, Source: "none",
			ChfUsd: zero, EurUsd: zero, GbpUsd: zero, BrlUsd: zero, CadUsd: zero,
		}, nil
	}

	source := "store"
	if _, covered, err := oracleHistoryForDay(day); err != nil {
		return nil, status.Errorf(codes.Internal, "oracle history: %v", err)
	} else if covered {
		source = "history"
	}

	return &types.QueryOracleFixingResponse{
		Day:    day,
		Found:  true,
		Source: source,
		ChfUsd: snap.ChfUsd,
		EurUsd: snap.EurUsd,
		GbpUsd: snap.GbpUsd,
		BrlUsd: snap.BrlUsd,
		CadUsd: snap.CadUsd,
	}, nil
}
