import type { RequestHandler } from './$types';

// Universal Links config — matches iOS app Team ID + bundle ID
// TODO(parent): replace TEAMID with actual Apple Team ID from provisioning profile
// before PROD launch. Debug/DEV bundle: com.vshage.app.dev ; Release/PROD: com.vshage.app
const AASA = {
  applinks: {
    details: [
      {
        appIDs: ['TEAMID.com.vshage.app', 'TEAMID.com.vshage.app.dev'],
        components: [
          { '/': '/*', comment: 'all afisha pages route to app if installed' }
        ]
      }
    ]
  }
};

export const GET: RequestHandler = () =>
  new Response(JSON.stringify(AASA), {
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'public, max-age=3600' }
  });
