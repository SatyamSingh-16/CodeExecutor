import { createBrowserRouter, RouteObject } from 'react-router-dom';
import { AppLayout } from '../components/shell/AppLayout';
import { HomePage } from '../pages/HomePage';
import { LoginPage } from '../pages/LoginPage';
import { RegisterPage } from '../pages/RegisterPage';
import { AppPage } from '../pages/AppPage';
import { NotFoundPage } from '../pages/NotFoundPage';

export const routes: RouteObject[] = [
  {
    path: '/',
    element: <AppLayout />,
    children: [
      {
        index: true,
        element: <HomePage />,
      },
      {
        path: 'login',
        element: <LoginPage />,
      },
      {
        path: 'register',
        element: <RegisterPage />,
      },
      {
        path: 'app',
        element: <AppPage />,
      },
      {
        path: '*',
        element: <NotFoundPage />,
      },
    ],
  },
];

export const router = createBrowserRouter(routes);
