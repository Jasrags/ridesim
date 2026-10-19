// Overpass query that produced extract-2026-10-19.json (run 2026-10-19 against overpass-api.de).
// Data © OpenStreetMap contributors, ODbL 1.0.
[out:json][timeout:120];
area["name"~"^(EPCOT|Epcot)$"]["tourism"="theme_park"]->.p;
(
  way["tourism"="theme_park"]["name"~"^(EPCOT|Epcot)$"];
  relation["tourism"="theme_park"]["name"~"^(EPCOT|Epcot)$"];
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
