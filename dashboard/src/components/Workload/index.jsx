import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Row, Col, Breadcrumb } from 'antd';
import { Link } from 'umi';
import './index.less';

export default class Workload extends React.PureComponent {
  render() {
    const { btnValue, pathname, title, crdInfo, state, settings, btnIsShow } = this.props.propsObj;
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Workloads</Breadcrumb.Item>
            <Breadcrumb.Item>{title}</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Row>
            <Col span="11">
              <div className="deployment">
                <Row>
                  <Col span="22">
                    <p className="title">{title}</p>
                    {crdInfo ? (
                      <p>
                        {crdInfo.apiVersion}
                        <span>,kind=</span>
                        {crdInfo.kind}
                      </p>
                    ) : (
                      <p />
                    )}
                  </Col>
                </Row>
                <p className="title">Configurable Settings:</p>
                {settings.map((item, index) => {
                  if (item.name === 'name') {
                    return <Fragment key={index.toString()} />;
                  }
                  return (
                    <Row key={index.toString()}>
                      <Col span="8">
                        <p>{item.name}</p>
                      </Col>
                      <Col span="16">
                        {
                          // eslint-disable-next-line consistent-return
                        }
                        <p>{item.default || item.usage}</p>
                      </Col>
                    </Row>
                  );
                })}
              </div>
              <Link to={{ pathname, state }} style={{ display: btnIsShow ? 'block' : 'none' }}>
                <Button type="primary" className="create-button">
                  {btnValue}
                </Button>
              </Link>
            </Col>
          </Row>
        </PageContainer>
      </div>
    );
  }
}
