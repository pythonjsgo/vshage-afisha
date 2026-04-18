<div class="scanlines" aria-hidden="true"></div>

<style>
  .scanlines {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 1000;
    background: repeating-linear-gradient(
      0deg,
      transparent 0,
      transparent 2px,
      rgba(255, 255, 255, 0.025) 2px,
      rgba(255, 255, 255, 0.025) 3px
    );
    animation: drift 40s linear infinite;
  }

  @keyframes drift {
    0%   { background-position: 0 0; }
    100% { background-position: 0 100px; }
  }
</style>
