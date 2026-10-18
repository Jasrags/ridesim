// Overpass query that produced extract-2026-10-18.json (run 2026-10-18 against overpass-api.de).
// Data © OpenStreetMap contributors, ODbL 1.0.
[out:json][timeout:90];
area["name"="Disney's Hollywood Studios"]["tourism"="theme_park"]->.p;
(
  way["tourism"="theme_park"]["name"="Disney's Hollywood Studios"];
  relation["tourism"="theme_park"]["name"="Disney's Hollywood Studios"];
  way(area.p)["highway"];
  nwr(area.p)["attraction"];
  nwr(area.p)["amenity"~"^(restaurant|fast_food|cafe|bar|ice_cream|theatre|toilets)$"];
  nwr(area.p)["shop"];
  node(area.p)["entrance"];
  nwr(area.p)["tourism"~"^(attraction|artwork)$"];
);
out body;
>;
out skel qt;
