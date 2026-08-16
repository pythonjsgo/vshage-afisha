export interface PublicEvent {
  id: string;
  /** Проставлен у событий веб-регистрации: карточка ведёт на /e/<slug>. */
  webreg_slug?: string;
  title: string;
  short_description?: string;
  description?: string;
  location?: string;
  start_time: string; // ISO
  end_time?: string;
  category?: string;
  tags: string[] | Record<string, unknown>;
  max_attendees?: number;
  attendee_count: number;
  photo_url?: string;
  status: 'published' | 'cancelled' | 'draft';
  registration_mode?: 'auto' | 'manual' | 'external';
  external_registration_url?: string;
  registration_deadline?: string;
  price_type?: 'free' | 'paid' | 'donation';
  price_min?: number;
  price_max?: number;
  currency?: string;
  city?: string;
  venue_name?: string;
  address?: string;
  online_url?: string;
  age_limit?: string;
  attendees_note?: string;
  is_featured: boolean;
  featured_position?: number;
  organizer_name?: string;
  organizer_photo?: string;
  photos?: string[];
}

export interface ListResult {
  featured: PublicEvent[];
  all: PublicEvent[];
  total: number;
}
