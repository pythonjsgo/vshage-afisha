export interface PublicEvent {
  id: string;
  title: string;
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
