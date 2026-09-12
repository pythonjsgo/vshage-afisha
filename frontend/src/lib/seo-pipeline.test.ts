import { describe, it, expect } from 'vitest';
import { eventJsonLd, eventMetaDescription } from './seo';
import type { PublicEvent } from './types';

const ev: PublicEvent = {id:'example',title:'Концерт',start_time:'2026-09-22T19:00:00+03:00',
  tags:[],attendee_count:0,is_featured:false,status:'published',registration_mode:'external',price_type:'free'};
describe('pipeline SEO facts',()=>{
  it('does not assert ticket inventory for an external event',()=>{
    const ld=eventJsonLd(ev,'https://afisha.vshage.app');
    expect(JSON.stringify(ld)).not.toContain('InStock');
    expect((ld.offers as Record<string,unknown>).price).toBe(0);
  });
  it('uses metadata without rewriting the visible description',()=>{
    const body='Полная программа концерта. '+ 'Подробности для посетителя. '.repeat(20);
    const card={...ev,description:body,seo_description:'Джазовый концерт в Москве.'};
    expect(eventMetaDescription(card)).toBe('Джазовый концерт в Москве.');
    expect(card.description).toBe(body);
    expect(eventMetaDescription({...card,seo_description:undefined}).length).toBeLessThanOrEqual(180);
  });
  it('reports cancellation rather than a scheduled event',()=>{
    expect(eventJsonLd({...ev,status:'cancelled'},'https://afisha.vshage.app').eventStatus).toBe('https://schema.org/EventCancelled');
  });
  it('does not invent a scheduled Event for an open-ended service',()=>{
    const ld=eventJsonLd({...ev,open_ended:true},'https://afisha.vshage.app');
    expect(ld['@type']).toBe('WebPage');
    expect(ld).not.toHaveProperty('startDate');
  });
});
