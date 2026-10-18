package osm

import "math"

// LatLon is a WGS84 coordinate in degrees.
type LatLon struct{ Lat, Lon float64 }

// Pt is a projected point in feet: X east and Y south of the projection origin (screen orientation,
// north up), matching the front end's SVG coordinates.
type Pt struct{ X, Y float64 }

const ftPerMeter = 1 / 0.3048

// Projection is a local equirectangular projection around a reference latitude. Over a theme park
// (about a mile across) its distance error is far below a foot per hundred feet.
type Projection struct {
	Origin      LatLon // maps to (0, 0); normally the north-west corner of the park
	ftPerDegLat float64
	ftPerDegLon float64
}

// NewProjection builds a projection with the given origin, using the meridian and parallel lengths at
// the origin's latitude (WGS84 series).
func NewProjection(origin LatLon) Projection {
	phi := origin.Lat * math.Pi / 180
	mLat := 111132.92 - 559.82*math.Cos(2*phi) + 1.175*math.Cos(4*phi) - 0.0023*math.Cos(6*phi)
	mLon := 111412.84*math.Cos(phi) - 93.5*math.Cos(3*phi) + 0.118*math.Cos(5*phi)
	return Projection{Origin: origin, ftPerDegLat: mLat * ftPerMeter, ftPerDegLon: mLon * ftPerMeter}
}

// Project maps a coordinate to feet east and south of the origin.
func (p Projection) Project(c LatLon) Pt {
	return Pt{(c.Lon - p.Origin.Lon) * p.ftPerDegLon, (p.Origin.Lat - c.Lat) * p.ftPerDegLat}
}

func dist(a, b Pt) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// closestOnSegment returns the point on segment ab closest to p and its parameter t in [0, 1].
func closestOnSegment(p, a, b Pt) (Pt, float64) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return a, 0
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	return Pt{a.X + t*dx, a.Y + t*dy}, t
}

func centroid(ps []Pt) Pt {
	var c Pt
	for _, p := range ps {
		c.X += p.X
		c.Y += p.Y
	}
	n := float64(len(ps))
	return Pt{c.X / n, c.Y / n}
}
