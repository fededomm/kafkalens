import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter, Routes, withComponentInputBinding } from '@angular/router';

const routes: Routes = [
  { path: '', loadComponent: () => import('./pages/clusters').then((m) => m.ClustersPage), title: 'Clusters · KafkaLens' },
  {
    path: 'c/:id',
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'overview' },
      { path: 'overview', loadComponent: () => import('./pages/overview').then((m) => m.OverviewPage), title: 'Overview · KafkaLens' },
      { path: 'topics', loadComponent: () => import('./pages/topics').then((m) => m.TopicsPage), title: 'Topics · KafkaLens' },
      { path: 'topics/:topic', loadComponent: () => import('./pages/topic').then((m) => m.TopicPage), title: 'Topic · KafkaLens' },
      { path: 'groups', loadComponent: () => import('./pages/groups').then((m) => m.GroupsPage), title: 'Consumer groups · KafkaLens' },
      { path: 'groups/:group', loadComponent: () => import('./pages/group').then((m) => m.GroupPage), title: 'Consumer group · KafkaLens' },
    ],
  },
  { path: '**', redirectTo: '' },
];

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes, withComponentInputBinding()),
  ],
};
