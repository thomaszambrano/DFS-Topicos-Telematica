package datanode

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/thomaszambrano/DFS-Topicos-Telematica/internal/pb"
)

// StartHeartbeat arranca el agente de heartbeat y bloquea hasta que ctx se cancela.
//
// El DataNode SIEMPRE inicia la conexión; el ControlNode nunca abre una hacia él
// (RN-12, RD-06). Esa dirección no es un detalle de implementación: permite que los
// DataNodes vivan detrás de NAT o en redes privadas, y hace que el ControlNode no
// necesite saber cómo alcanzarlos. Las órdenes viajan de vuelta en la respuesta.
func (d *DataNode) StartHeartbeat(ctx context.Context, controlAddr string, interval time.Duration) error {
	// grpc.NewClient no conecta de inmediato: establece y reestablece la conexión
	// según haga falta. Un ControlNode caído no tumba al DataNode, y cuando vuelve
	// el siguiente heartbeat lo encuentra sin que nadie reintente a mano.
	conn, err := grpc.NewClient(controlAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewDataNodeServiceClient(conn)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[%s] heartbeat hacia %s cada %s", d.NodeID, controlAddr, interval)
	d.beat(ctx, client) // el primero va de inmediato, sin esperar un tick

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.beat(ctx, client)
		}
	}
}

// beat envía un reporte y ejecuta las órdenes que vengan de vuelta.
func (d *DataNode) beat(ctx context.Context, client pb.DataNodeServiceClient) {
	inventory, err := d.Store.Inventory()
	if err != nil {
		log.Printf("[%s] no pude leer el inventario: %v", d.NodeID, err)
		return
	}
	ids := make([]string, len(inventory))
	for i, id := range inventory {
		ids[i] = string(id)
	}
	used, capacity, _ := d.Store.Stats()

	// El reporte lleva el inventario COMPLETO, no un delta. Es lo que permite al
	// ControlNode reconstruir el mapa de ubicaciones desde cero tras un reinicio
	// sin leer su propio disco (RN-03), y lo que hace que ese mapa sea estado
	// blando: el disco de este nodo es la única fuente de verdad.
	report := &pb.NodeReport{
		NodeId:             d.NodeID,
		AdvertisedEndpoint: d.Endpoint,
		CapacityBytes:      capacity,
		UsedBytes:          used,
		ChunkIds:           ids,
		InFlight:           d.inFlight.Load(),
		TimestampUnixMs:    time.Now().UnixMilli(),
		Draining:           false, // el drenado es del Hito 3 (RD-11)
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	commands, err := client.Heartbeat(callCtx, report)
	if err != nil {
		log.Printf("[%s] heartbeat falló: %v", d.NodeID, err)
		return
	}
	for _, cmd := range commands.GetCommands() {
		d.execute(cmd)
	}
}

// execute atiende una orden recibida en la respuesta al heartbeat.
//
// En el Hito 2 la lista llega siempre vacía. El switch existe para que la
// dirección de la comunicación quede establecida desde ahora y el Hito 3 solo
// tenga que rellenar los casos.
func (d *DataNode) execute(cmd *pb.Command) {
	switch cmd.GetType() {
	case pb.CommandType_COMMAND_TYPE_DELETE_CHUNK:
		log.Printf("[%s] orden DELETE %s", d.NodeID, cmd.GetChunkId())
	case pb.CommandType_COMMAND_TYPE_COPY_CHUNK:
		log.Printf("[%s] orden COPY %s -> %s (Hito 3)", d.NodeID, cmd.GetChunkId(), cmd.GetTargetEndpoint())
	case pb.CommandType_COMMAND_TYPE_REPORT_INVENTORY:
		log.Printf("[%s] orden REPORT_INVENTORY", d.NodeID)
	default:
		log.Printf("[%s] orden desconocida: %v", d.NodeID, cmd.GetType())
	}
}
