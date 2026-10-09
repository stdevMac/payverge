const { opsAssistant } = require('./prod_prompts');

describe('prod_prompts ops assistant builder', () => {
  it('opsAssistant embeds active tab context', () => {
    const msgs = opsAssistant({ vars: { LANG: 'en', active_tab: 'menu', user_message: 'Help' } });
    expect(msgs[0].content).toMatch(/Active tab: menu/);
    expect(msgs[1].content).toBe('Help');
  });
});
