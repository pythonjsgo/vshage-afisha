import type { PageServerLoad } from './$types';
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

export type PlaceCard = {
	id: string;
	name: string;
	kind: string;
	district: string;
	lat: number;
	lon: number;
	address?: string;
	maps_url?: string;
	talk_friendly: number;
	laptop_friendly: number;
	outlets_score: number;
	rating_avg?: number | null;
	rating_count: number;
	quality_score: number;
	signal_tags: string[];
	blurb?: string;
};

type CatalogFile = { count: number; places: PlaceCard[] };

/** Load published catalog from static JSON baked at build/export time. */
export const load: PageServerLoad = async () => {
	const candidates = [
		join(process.cwd(), 'static', 'places-catalog.json'),
		join(process.cwd(), 'src', 'lib', 'data', 'places-catalog.json'),
		// adapter-node may run from build/
		join(process.cwd(), '..', 'static', 'places-catalog.json')
	];
	let data: CatalogFile = { count: 0, places: [] };
	for (const p of candidates) {
		try {
			const raw = await readFile(p, 'utf-8');
			data = JSON.parse(raw) as CatalogFile;
			break;
		} catch {
			/* try next */
		}
	}
	// Optional remote overlay (core-api) when PUBLIC_PLACES_API is set at runtime
	const remote = process.env.PUBLIC_PLACES_API || process.env.PLACES_API_URL;
	if (remote) {
		try {
			const res = await fetch(`${remote.replace(/\/$/, '')}/api/places?limit=2000`);
			if (res.ok) {
				const j = (await res.json()) as {
					total?: number;
					places?: Array<Record<string, unknown>>;
				};
				if (j.places?.length) {
					data = {
						count: j.total ?? j.places.length,
						places: j.places.map((p) => ({
							id: String(p.VenueID ?? p.venue_id ?? p.id ?? ''),
							name: String(p.Name ?? p.name ?? ''),
							kind: String(p.Kind ?? p.kind ?? 'other'),
							district: String(p.District ?? p.district ?? ''),
							lat: Number(p.Lat ?? p.lat ?? 0),
							lon: Number(p.Lon ?? p.lon ?? 0),
							address: String(p.Address ?? p.address ?? ''),
							maps_url: String(p.MapsURL ?? p.maps_url ?? ''),
							talk_friendly: Number(p.TalkFriendly ?? p.talk_friendly ?? 0.5),
							laptop_friendly: Number(p.LaptopFriendly ?? p.laptop_friendly ?? 0.5),
							outlets_score: Number(p.OutletsScore ?? p.outlets_score ?? 0.5),
							rating_avg: (p.RatingAvg ?? p.rating_avg) as number | null | undefined,
							rating_count: Number(p.RatingCount ?? p.rating_count ?? 0),
							quality_score: Number(p.QualityScore ?? p.quality_score ?? 0.5),
							signal_tags: (p.SignalTags ?? p.signal_tags ?? []) as string[],
							blurb: String(p.EmbedText ?? p.embed_text ?? p.blurb ?? '')
						}))
					};
				}
			}
		} catch {
			/* keep static */
		}
	}
	return {
		total: data.count || data.places.length,
		places: data.places || []
	};
};
