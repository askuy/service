package cargo

import (
	"time"

	"github.com/feixiao/learning/gokit/shipping/location"
	"github.com/feixiao/learning/gokit/shipping/voyage"
)

// Delivery is the actual transportation of the cargo, as opposed to the
// customer requirement (RouteSpecification) and the plan (Itinerary).
// 跟Cargo是一对一的关系，所属于某个具体的Cargo
type Delivery struct {
	Itinerary               Itinerary				// 规定的运输起点、终点以及到达期限，跟Cargo中Itinerary有重复
	RouteSpecification     RouteSpecification   // 中途经历的路线，跟Cargo中Itinerary有重复
	RoutingStatus           RoutingStatus		// 路由选择状态 NotRouted、Routed和Misrouted
	TransportStatus         TransportStatus		// 运输状态 NotReceived、InPort、OnboardCarrier、Claimed、Unknown
	NextExpectedActivity   HandlingActivity
	LastEvent               HandlingEvent			// 最处理的事件
	LastKnownLocation       location.UNLocode	// 最近一个已知位置
	CurrentVoyage           voyage.Number			// 当前所在航线
	ETA                     time.Time
	IsMisdirected           bool					// 是否出现地址错误问题
	IsUnloadedAtDestination bool					// 是否出现在目的地没有卸载的情况
}

// UpdateOnRouting creates a new delivery snapshot to reflect changes in
// routing, i.e. when the route specification or the itinerary has changed but
// no additional handling of the cargo has been performed.
func (d Delivery) UpdateOnRouting(rs RouteSpecification, itinerary Itinerary) Delivery {
	return newDelivery(d.LastEvent, itinerary, rs)
}

// IsOnTrack checks if the delivery is on track.
func (d Delivery) IsOnTrack() bool {
	return d.RoutingStatus == Routed && !d.IsMisdirected
}

// DeriveDeliveryFrom creates a new delivery snapshot based on the complete
// handling history of a cargo, as well as its route specification and
// itinerary.
func DeriveDeliveryFrom(rs RouteSpecification, itinerary Itinerary, history HandlingHistory) Delivery {
	lastEvent, _ := history.MostRecentlyCompletedEvent()
	return newDelivery(lastEvent, itinerary, rs)
}

// newDelivery creates a up-to-date delivery based on an handling event,
// itinerary and a route specification.
func newDelivery(lastEvent HandlingEvent, itinerary Itinerary, rs RouteSpecification) Delivery {
	var (
		routingStatus           = calculateRoutingStatus(itinerary, rs)
		transportStatus         = calculateTransportStatus(lastEvent)
		lastKnownLocation       = calculateLastKnownLocation(lastEvent)
		isMisdirected           = calculateMisdirectedStatus(lastEvent, itinerary)
		isUnloadedAtDestination = calculateUnloadedAtDestination(lastEvent, rs)
		currentVoyage           = calculateCurrentVoyage(transportStatus, lastEvent)
	)

	d := Delivery{
		LastEvent:               lastEvent,
		Itinerary:               itinerary,
		RouteSpecification:      rs,
		RoutingStatus:           routingStatus,
		TransportStatus:         transportStatus,
		LastKnownLocation:       lastKnownLocation,
		IsMisdirected:           isMisdirected,
		IsUnloadedAtDestination: isUnloadedAtDestination,
		CurrentVoyage:           currentVoyage,
	}

	d.NextExpectedActivity = calculateNextExpectedActivity(d)
	d.ETA = calculateETA(d)

	return d
}

// Below are internal functions used when creating a new delivery.

// 获取路由选择状态
func calculateRoutingStatus(itinerary Itinerary, rs RouteSpecification) RoutingStatus {
	if itinerary.Legs == nil {
		return NotRouted
	}

	// Itinerary和RouteSpecification的起点终点相同说明已经被分配
	if rs.IsSatisfiedBy(itinerary) {
		return Routed
	}

	return Misrouted
}

func calculateMisdirectedStatus(event HandlingEvent, itinerary Itinerary) bool {
	if event.Activity.Type == NotHandled {
		return false
	}

	return !itinerary.IsExpected(event)
}

func calculateUnloadedAtDestination(event HandlingEvent, rs RouteSpecification) bool {
	if event.Activity.Type == NotHandled {
		return false
	}

	return event.Activity.Type == Unload && rs.Destination == event.Activity.Location
}

func calculateTransportStatus(event HandlingEvent) TransportStatus {
	switch event.Activity.Type {
	case NotHandled:
		return NotReceived
	case Load:
		return OnboardCarrier
	case Unload:
		return InPort
	case Receive:
		return InPort
	case Customs:
		return InPort
	case Claim:
		return Claimed
	}
	return Unknown
}

func calculateLastKnownLocation(event HandlingEvent) location.UNLocode {
	return event.Activity.Location
}

func calculateNextExpectedActivity(d Delivery) HandlingActivity {
	if !d.IsOnTrack() {
		return HandlingActivity{}
	}

	switch d.LastEvent.Activity.Type {
	case NotHandled:
		return HandlingActivity{Type: Receive, Location: d.RouteSpecification.Origin}
	case Receive:
		l := d.Itinerary.Legs[0]
		return HandlingActivity{Type: Load, Location: l.LoadLocation, VoyageNumber: l.VoyageNumber}
	case Load:
		for _, l := range d.Itinerary.Legs {
			if l.LoadLocation == d.LastEvent.Activity.Location {
				return HandlingActivity{Type: Unload, Location: l.UnloadLocation, VoyageNumber: l.VoyageNumber}
			}
		}
	case Unload:
		for i, l := range d.Itinerary.Legs {
			if l.UnloadLocation == d.LastEvent.Activity.Location {
				if i < len(d.Itinerary.Legs)-1 {
					return HandlingActivity{Type: Load, Location: d.Itinerary.Legs[i+1].LoadLocation, VoyageNumber: d.Itinerary.Legs[i+1].VoyageNumber}
				}

				return HandlingActivity{Type: Claim, Location: l.UnloadLocation}
			}
		}
	}

	return HandlingActivity{}
}

func calculateCurrentVoyage(transportStatus TransportStatus, event HandlingEvent) voyage.Number {
	if transportStatus == OnboardCarrier && event.Activity.Type != NotHandled {
		return event.Activity.VoyageNumber
	}

	return voyage.Number("")
}

func calculateETA(d Delivery) time.Time {
	if !d.IsOnTrack() {
		return time.Time{}
	}

	return d.Itinerary.FinalArrivalTime()
}
