package bridge

import (
	"context"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/definition"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

func ResolveGraph(def *definition.GraphDef, infra Infra) (*lifecycle.Graph, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ApplyBridgeReducerDefaults(def)
	factory := NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    MessageReducer,
			"reflections": ReflectionReducer,
			"total_usage": UsageReducer,
		},
	}
	return definition.Resolve(def, factory.Create, definition.DefaultRouterCreator(), opts)
}

func GenerateAndResolveGraph(ctx context.Context, prov provider.Provider, bus *observe.EventBus, modelID, structure string, infra Infra) (*lifecycle.Graph, error) {
	observe.TraceCtx(ctx, "lifecycle/bridge", "GenerateAndResolveGraph", "enter")
	defer observe.TraceCtx(ctx, "lifecycle/bridge", "GenerateAndResolveGraph", "exit")
	def, err := GenerateGraph(ctx, prov, bus, modelID, structure)
	if err != nil {
		observe.TraceCtx(ctx, "lifecycle/bridge", "GenerateAndResolveGraph", "if: err != nil")
		return nil, err
	}
	return ResolveGraph(def, infra)
}

func ApplyBridgeReducerDefaults(def *definition.GraphDef) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if def.Graph.Reducers == nil {
		observe.GlobalTrace("if: def.Graph.Reducers == nil")
		def.Graph.Reducers = make(map[string]string)
	}
	if _, ok := def.Graph.Reducers[KeyTotalUsage]; !ok {
		observe.GlobalTrace("if: KeyTotalUsage reducer missing")
		def.Graph.Reducers[KeyTotalUsage] = "total_usage"
	}
	if _, ok := def.Graph.Reducers[KeyTurnCount]; !ok {
		observe.GlobalTrace("if: KeyTurnCount reducer missing")
		def.Graph.Reducers[KeyTurnCount] = "sum"
	}
}
