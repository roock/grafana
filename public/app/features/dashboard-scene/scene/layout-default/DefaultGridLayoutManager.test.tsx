import { SceneGridItemLike, SceneGridLayout, SceneGridRow, SceneQueryRunner, VizPanel } from '@grafana/scenes';

import { findVizPanelByKey } from '../../utils/utils';
import { DashboardGridItem } from '../DashboardGridItem';

import { DefaultGridLayoutManager } from './DefaultGridLayoutManager';

describe('DefaultGridLayoutManager', () => {
  describe('getVizPanels', () => {
    it('Should return all panels', () => {
      const { manager } = setup();
      const vizPanels = manager.getVizPanels();

      expect(vizPanels.length).toBe(4);
      expect(vizPanels[0].state.title).toBe('Panel A');
      expect(vizPanels[1].state.title).toBe('Panel B');
      expect(vizPanels[2].state.title).toBe('Panel C');
      expect(vizPanels[3].state.title).toBe('Panel D');
    });

    it('Should return an empty array when scene has no panels', () => {
      const { manager } = setup({ gridItems: [] });
      const vizPanels = manager.getVizPanels();
      expect(vizPanels.length).toBe(0);
    });
  });

  describe('getNextPanelId', () => {
    it('should get next panel id in a simple 3 panel layout', () => {
      const { manager } = setup();
      const id = manager.getNextPanelId();

      expect(id).toBe(4);
    });

    it('should return 1 if no panels are found', () => {
      const { manager } = setup({ gridItems: [] });
      const id = manager.getNextPanelId();

      expect(id).toBe(1);
    });
  });

  describe('removeElement', () => {
    it('should remove element', () => {
      const { manager, grid } = setup();

      expect(grid.state.children.length).toBe(3);

      manager.removeElement(grid.state.children[0] as DashboardGridItem);

      expect(grid.state.children.length).toBe(2);
    });
  });

  describe('addPanel', () => {
    it('Should add a new panel', () => {
      const { manager, grid } = setup();

      const vizPanel = new VizPanel({
        title: 'Panel Title',
        key: 'panel-55',
        pluginId: 'timeseries',
        $data: new SceneQueryRunner({ key: 'data-query-runner', queries: [{ refId: 'A' }] }),
      });

      manager.addPanel(vizPanel);

      const panel = findVizPanelByKey(manager, 'panel-55')!;
      const gridItem = panel.parent as DashboardGridItem;

      expect(panel).toBeDefined();
      expect(gridItem.state.y).toBe(0);
    });
  });

  describe('addNewRow', () => {
    it('Should create and add a new row to the dashboard', () => {
      const { manager, grid } = setup();
      const row = manager.addNewRow();

      expect(grid.state.children.length).toBe(2);
      expect(row.state.key).toBe('panel-4');
      expect(row.state.children[0].state.key).toBe('griditem-1');
      expect(row.state.children[1].state.key).toBe('griditem-2');
    });

    it('Should create a row and add all panels in the dashboard under it', () => {
      const { manager, grid } = setup({
        gridItems: [
          new DashboardGridItem({
            key: 'griditem-1',
            x: 0,
            body: new VizPanel({
              title: 'Panel A',
              key: 'panel-1',
              pluginId: 'table',
              $data: new SceneQueryRunner({ key: 'data-query-runner', queries: [{ refId: 'A' }] }),
            }),
          }),
          new DashboardGridItem({
            key: 'griditem-2',
            body: new VizPanel({
              title: 'Panel B',
              key: 'panel-2',
              pluginId: 'table',
            }),
          }),
        ],
      });

      const row = manager.addNewRow();

      expect(grid.state.children.length).toBe(1);
      expect(row.state.children.length).toBe(2);
    });
  });
});

interface TestOptions {
  gridItems: SceneGridItemLike[];
}

function setup(options?: TestOptions) {
  const gridItems = options?.gridItems ?? [
    new DashboardGridItem({
      key: 'griditem-1',
      x: 0,
      body: new VizPanel({
        title: 'Panel A',
        key: 'panel-1',
        pluginId: 'table',
        $data: new SceneQueryRunner({ key: 'data-query-runner', queries: [{ refId: 'A' }] }),
      }),
    }),
    new DashboardGridItem({
      key: 'griditem-2',
      body: new VizPanel({
        title: 'Panel B',
        key: 'panel-2',
        pluginId: 'table',
      }),
    }),
    new SceneGridRow({
      key: 'panel-3',
      title: 'row',
      children: [
        new DashboardGridItem({
          body: new VizPanel({
            title: 'Panel C',
            key: 'panel-2-clone-2',
            pluginId: 'table',
          }),
        }),
        new DashboardGridItem({
          body: new VizPanel({
            title: 'Panel D',
            key: 'panel-2-clone-2',
            pluginId: 'table',
          }),
        }),
      ],
    }),
  ];

  const grid = new SceneGridLayout({ children: gridItems });
  const manager = new DefaultGridLayoutManager({ layout: grid });

  return { manager, grid };
}
